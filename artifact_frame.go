package qpool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

const (
	artifactAttrTTLNs       = "ttl_ns"
	artifactAttrSenderID    = "sender_id"
	artifactAttrReceiverID  = "receiver_id"
	artifactAttrMessageType = "message_type"
)

/*
ArtifactError returns the terminal error stored on an artifact, if any.
*/
func ArtifactError(artifact *datura.Artifact) error {
	if artifact == nil || !artifact.HasError() {
		return nil
	}

	artifactErr, err := artifact.Error()

	if err != nil {
		return err
	}

	message, err := artifactErr.Message_()

	if err != nil {
		return err
	}

	if message == "" {
		return errors.New("qpool: artifact error")
	}

	return errors.New(message)
}

/*
ArtifactValue decodes the artifact payload into typed job output.
*/
func ArtifactValue[T any](artifact *datura.Artifact) (T, error) {
	var zero T

	if artifactErr := ArtifactError(artifact); artifactErr != nil {
		return zero, artifactErr
	}

	payload := artifact.DecryptPayload()

	switch any(zero).(type) {
	case string:
		return any(string(payload)).(T), nil
	case []byte:
		return any(payload).(T), nil
	default:
		var typed T

		if len(payload) == 0 {
			return zero, nil
		}

		if err := json.Unmarshal(payload, &typed); err != nil {
			return zero, fmt.Errorf("qpool: decode artifact payload: %w", err)
		}

		return typed, nil
	}
}

func artifactTTL(artifact *datura.Artifact) time.Duration {
	if artifact == nil {
		return 0
	}

	raw := datura.Peek[string](artifact, artifactAttrTTLNs)

	if raw == "" {
		return 0
	}

	nanoseconds, err := strconv.ParseInt(raw, 10, 64)

	if err != nil {
		return 0
	}

	return time.Duration(nanoseconds)
}

func encodePayload(value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		payload, err := json.Marshal(value)

		if err != nil {
			return nil, fmt.Errorf("qpool: encode artifact payload: %w", err)
		}

		return payload, nil
	}
}

func newResultArtifact(
	jobID string,
	value any,
	ttl time.Duration,
) (*datura.Artifact, error) {
	artifact := datura.Acquire("qpool", datura.Artifact_Type_json)

	if artifact == nil {
		return nil, errors.New("qpool: artifact acquire failed")
	}

	payload, err := encodePayload(value)

	if err != nil {
		return nil, err
	}

	if err := artifact.SetScope(jobID); err != nil {
		return nil, err
	}

	artifact.WithPayload(payload)
	artifact.Poke(artifactAttrTTLNs, strconv.FormatInt(int64(ttl), 10))

	return artifact, nil
}

func newErrorArtifact(
	jobID string,
	terminalErr error,
	ttl time.Duration,
) (*datura.Artifact, error) {
	artifact, err := newResultArtifact(jobID, nil, ttl)

	if err != nil {
		return nil, err
	}

	artifactErr, err := datura.NewArtifact_Error(artifact.Segment())

	if err != nil {
		return nil, err
	}

	if terminalErr == nil {
		terminalErr = errors.New("qpool: job failed")
	}

	if err := artifactErr.SetMessage_(terminalErr.Error()); err != nil {
		return nil, err
	}

	artifact.SetError(artifactErr)

	return artifact, nil
}

func cloneArtifact(source *datura.Artifact) *datura.Artifact {
	if source == nil {
		return nil
	}

	origin, _ := source.Origin()
	cloned := datura.Acquire(origin, source.Type())

	if cloned == nil {
		return nil
	}

	raw := source.Pack()

	if _, err := cloned.Write(raw); err != nil {
		return nil
	}

	return cloned
}

/*
NewBusArtifact builds a broadcast artifact for channel fan-out.
*/
func NewBusArtifact(
	senderID string,
	receiverID string,
	messageType string,
	value any,
	ttl time.Duration,
) (*datura.Artifact, error) {
	artifact, err := newResultArtifact(messageType, value, ttl)

	if err != nil {
		return nil, err
	}

	if err := artifact.SetOrigin(senderID); err != nil {
		return nil, err
	}

	if err := artifact.SetDestination(receiverID); err != nil {
		return nil, err
	}

	if err := artifact.SetRole(messageType); err != nil {
		return nil, err
	}

	artifact.Poke(artifactAttrSenderID, senderID)
	artifact.Poke(artifactAttrReceiverID, receiverID)
	artifact.Poke(artifactAttrMessageType, messageType)

	return artifact, nil
}

/*
BusMessageType returns the routed message type from a bus artifact.
*/
func BusMessageType(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}

	messageType, err := artifact.Role()

	if err == nil && messageType != "" {
		return messageType
	}

	return datura.Peek[string](artifact, artifactAttrMessageType)
}

/*
BusMessageValue decodes the payload stored on a bus artifact.
*/
func BusMessageValue(artifact *datura.Artifact) (any, error) {
	if artifact == nil {
		return nil, errors.New("qpool: nil bus artifact")
	}

	payload := artifact.DecryptPayload()
	if len(payload) == 0 {
		return nil, nil
	}

	var decoded any

	if err := sonic.Unmarshal(payload, &decoded); err != nil {
		return string(payload), nil
	}

	return decoded, nil
}

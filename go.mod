module github.com/theapemachine/qpool

go 1.26.1

replace (
	github.com/bytedance/sonic => ../sonic
	github.com/theapemachine/datura => ../datura
)

// replace github.com/theapemachine/errnie => ../errnie

require (
	github.com/google/uuid v1.6.0
	github.com/smarty/go-disruptor v0.5.0
	github.com/smartystreets/goconvey v1.8.1
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.2 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	github.com/elastic/elastic-transport-go/v8 v8.11.0 // indirect
	github.com/elastic/go-elasticsearch/v9 v9.4.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.71.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

require (
	github.com/bytedance/sonic v1.15.2
	github.com/gopherjs/gopherjs v1.20.2 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/phuslu/log v1.0.124 // indirect
	github.com/smarty/assertions v1.16.0 // indirect
	github.com/theapemachine/datura v1.2.4
	github.com/theapemachine/errnie v1.2.5
)

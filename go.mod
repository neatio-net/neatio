module github.com/neatio-net/neatio

go 1.21

require (
	github.com/allegro/bigcache v1.2.1
	github.com/aristanetworks/goarista v0.0.0-20210715113802-a1396632fc37
	github.com/btcsuite/btcd v0.22.0-beta
	github.com/cespare/cp v1.1.1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/docker/docker v20.10.7+incompatible
	github.com/fatih/color v1.12.0
	github.com/gizak/termui v2.3.0+incompatible
	github.com/go-stack/stack v1.8.0
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4
	github.com/hashicorp/golang-lru v0.5.4
	github.com/huin/goupnp v1.0.2
	github.com/influxdata/influxdb v1.9.3
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/karalabe/hid v1.0.0
	github.com/mattn/go-colorable v0.1.8
	github.com/naoina/toml v0.1.1
	github.com/neatio-net/bls-go v1.1.0 // OK
	github.com/neatio-net/common-go v1.3.2 // OK
	github.com/neatio-net/config-go v1.1.1 // OK
	github.com/neatio-net/crypto-go v1.2.2 // OK
	github.com/neatio-net/db-go v1.2.1 // OK
	github.com/neatio-net/events-go v1.1.1 // OK
	github.com/neatio-net/flock-go v1.0.0 // OK
	github.com/neatio-net/merkle-go v1.1.1 // OK
	github.com/neatio-net/set-go v1.0.0 // OK
	github.com/neatio-net/wire-go v1.1.1 // OK
	github.com/pborman/uuid v1.2.1
	github.com/peterh/liner v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/rjeczalik/notify v0.9.2
	github.com/robertkrimen/otto v0.0.0-20210614181706-373ff5438452
	github.com/rs/cors v1.8.0
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/net v0.0.0-20210716203947-853a461950ff
	golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/olebedev/go-duktape.v3 v3.0.0-20210326210528-650f7c854440
	gopkg.in/urfave/cli.v1 v1.20.0
)

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/andreyvit/diff v0.0.0-20170406064948-c7f18ee00883 // indirect
	github.com/apache/arrow/go/arrow v0.0.0-20200923215132-ac86123a3f01 // indirect
	github.com/benbjohnson/immutable v0.2.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/google/flatbuffers v2.0.0+incompatible // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/influxdata/flux v0.120.1 // indirect
	github.com/influxdata/influxql v1.1.1-0.20210223160523-b6ab99450c93 // indirect
	github.com/jmhodges/levigo v1.0.0 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/maruel/panicparse v1.6.1 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/neatio-net/data-go v1.1.2 // indirect; OK
	github.com/neatio-net/ed25519 v1.0.1 // indirect
	github.com/neatio-net/ed25519-go v1.0.1 // indirect; OK
	github.com/neatio-net/log15-go v1.1.1 // indirect; OK
	github.com/neatio-net/logger-go v1.1.1 // indirect; OK
	github.com/nsf/termbox-go v1.1.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.9.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.18.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/uber/jaeger-client-go v2.28.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xlab/treeprint v0.0.0-20180616005107-d6fb6747feb6 // indirect
	go.uber.org/atomic v1.6.0 // indirect
	go.uber.org/multierr v1.5.0 // indirect
	go.uber.org/zap v1.14.1 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	gotest.tools/v3 v3.0.3 // indirect
)

require (
	github.com/intfoundation/go-common v1.0.3 // indirect
	github.com/intfoundation/intchain v0.0.0-20220727031208-4316ad31ca73 // indirect
	github.com/neatlib/bls-go v1.1.0 // indirect
)

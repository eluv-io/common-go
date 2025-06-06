module github.com/eluv-io/common-go

go 1.21.0

require (
	github.com/PaesslerAG/gval v1.1.2
	github.com/PaesslerAG/jsonpath v0.1.1
	github.com/beevik/etree v1.1.0
	github.com/davecgh/go-spew v1.1.1
	github.com/djherbis/times v1.6.0
	github.com/eluv-io/apexlog-go v1.9.1-elv4
	github.com/eluv-io/errors-go v1.0.3
	github.com/eluv-io/inject-go v1.0.2
	github.com/eluv-io/log-go v1.0.4
	github.com/eluv-io/utc-go v1.0.1
	github.com/ethereum/go-ethereum v1.10.19
	github.com/fxamacker/cbor/v2 v2.8.0
	github.com/gammazero/deque v0.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/gin-gonic/gin v1.7.7
	github.com/hashicorp/golang-lru v0.5.5-0.20210104140557-80c98217689d
	github.com/maruel/panicparse/v2 v2.3.1
	github.com/mattn/go-runewidth v0.0.9
	github.com/mitchellh/mapstructure v1.4.3
	github.com/modern-go/gls v0.0.0-20220109145502-612d0167dce5
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-varint v0.0.6
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0
	github.com/ricochet2200/go-disk-usage/du v0.0.0-20210707232629-ac9918953285
	github.com/satori/go.uuid v1.2.0
	github.com/smartystreets/goconvey v1.8.1
	github.com/spf13/afero v1.3.2
	github.com/stretchr/testify v1.8.4
	github.com/ugorji/go/codec v1.1.7
	go.uber.org/atomic v1.9.0
	golang.org/x/exp v0.0.0-20220426173459-3bcf042a4bf5
	golang.org/x/text v0.14.0
)

require (
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.0 // indirect
	github.com/deckarep/golang-set v1.8.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/eluv-io/stack v1.8.2 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-ole/go-ole v1.2.1 // indirect
	github.com/go-playground/locales v0.13.0 // indirect
	github.com/go-playground/universal-translator v0.17.0 // indirect
	github.com/go-playground/validator/v10 v10.4.1 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/leodido/go-urn v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/rjeczalik/notify v0.9.3 // indirect
	github.com/shirou/gopsutil v3.21.4-0.20210419000835-c7a38de76ee5+incompatible // indirect
	github.com/smarty/assertions v1.15.0 // indirect
	github.com/tklauser/go-sysconf v0.3.5 // indirect
	github.com/tklauser/numcpus v0.2.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.16.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/fxamacker/cbor/v2 v2.8.0 => github.com/eluv-io/cbor/v2 v2.8.1-0.20250506081522-e7b11bfa1dad
	github.com/modern-go/gls => github.com/eluv-io/gls v1.0.0-elv1
	github.com/spf13/afero => github.com/eluv-io/afero v1.11.1-0.20240924184135-9fbf4dcfd6f0
)

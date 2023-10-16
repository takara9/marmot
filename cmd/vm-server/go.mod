module vm-server

go 1.19

replace (
	github.com/takara9/marmot/pkg/config => ../../pkg/config
	github.com/takara9/marmot/pkg/db => ../../pkg/db
	github.com/takara9/marmot/pkg/lvm => ../../pkg/lvm
	github.com/takara9/marmot/pkg/util => ../../pkg/util
	github.com/takara9/marmot/pkg/virt => ../../pkg/virt
)

require (
	github.com/gin-gonic/gin v1.8.2
	github.com/takara9/marmot/pkg/config v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/db v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/util v0.0.0-00010101000000-000000000000
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-playground/validator/v10 v10.11.1 // indirect
	github.com/goccy/go-json v0.9.11 // indirect
	github.com/gofrs/flock v0.7.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/takara9/lvm v0.0.0-20230311125634-cefe325205bd // indirect
	github.com/takara9/marmot/pkg/lvm v0.0.0-00010101000000-000000000000 // indirect
	github.com/takara9/marmot/pkg/virt v0.0.0-00010101000000-000000000000 // indirect
	github.com/ugorji/go/codec v1.2.7 // indirect
	go.etcd.io/etcd/api/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/v3 v3.5.7 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/crypto v0.0.0-20211215153901-e495a2d5b3d3 // indirect
	golang.org/x/net v0.4.0 // indirect
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/text v0.5.0 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/grpc v1.41.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/freddierice/go-losetup.v1 v1.0.0-20170407175016-fc9adea44124 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	libvirt.org/go/libvirt v1.9000.0 // indirect
)

module hvadm

go 1.19

replace (
	github.com/takara9/marmot/pkg/config => ../../pkg/config
	github.com/takara9/marmot/pkg/db => ../../pkg/db
	github.com/takara9/marmot/pkg/lvm => ../../pkg/lvm
	github.com/takara9/marmot/pkg/virt => ../../pkg/virt
)

require github.com/takara9/marmot/pkg/db v0.0.0-00010101000000-000000000000

require (
	github.com/kr/text v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require github.com/takara9/marmot/pkg/config v0.0.0-20231028131622-506aa699df46

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.3.0 // indirect
	go.etcd.io/etcd/api/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/v3 v3.5.7 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/grpc v1.41.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

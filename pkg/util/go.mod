module github.com/takara9/marmot/pkg/util

go 1.19

replace (
	github.com/takara9/marmot/pkg/config => ../config
	github.com/takara9/marmot/pkg/db => ../db
	github.com/takara9/marmot/pkg/lvm => ../lvm
	github.com/takara9/marmot/pkg/util => ../util
	github.com/takara9/marmot/pkg/virt => ../virt
)

require (
	github.com/takara9/marmot/pkg/config v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/db v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/lvm v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/virt v0.0.0-00010101000000-000000000000
	go.etcd.io/etcd/client/v3 v3.5.7
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/go-yaml/yaml v2.1.0+incompatible // indirect
	github.com/gofrs/flock v0.7.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/takara9/lvm v0.0.0-20230311125634-cefe325205bd // indirect
	go.etcd.io/etcd/api/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.7 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40 // indirect
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/grpc v1.41.0 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/freddierice/go-losetup.v1 v1.0.0-20170407175016-fc9adea44124 // indirect
	libvirt.org/go/libvirt v1.9000.0 // indirect
)

module github.com/takara9/marmot/pkg/util

go 1.21.3

replace (
	github.com/takara9/marmot/pkg/config => ../config
	github.com/takara9/marmot/pkg/db => ../db
	github.com/takara9/marmot/pkg/lvm => ../lvm
	github.com/takara9/marmot/pkg/util => ../util
	github.com/takara9/marmot/pkg/virt => ../virt
)

require (
	github.com/takara9/marmot/pkg/config v0.0.0-20231029092358-6bbe00b9567a
	github.com/takara9/marmot/pkg/db v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/lvm v0.0.0-00010101000000-000000000000
	github.com/takara9/marmot/pkg/virt v0.0.0-00010101000000-000000000000
	go.etcd.io/etcd/client/v3 v3.5.10
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/gofrs/flock v0.7.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/takara9/lvm v0.0.0-20230311125634-cefe325205bd // indirect
	go.etcd.io/etcd/api/v3 v3.5.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.10 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.17.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/grpc v1.58.3 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/freddierice/go-losetup.v1 v1.0.0-20170407175016-fc9adea44124 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	libvirt.org/go/libvirt v1.9000.0 // indirect
)

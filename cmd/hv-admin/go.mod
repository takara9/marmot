module main

go 1.24.4

require marmot.io/util v0.8.6

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/gofrs/flock v0.7.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/labstack/echo/v4 v4.12.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/takara9/lvm v0.0.0-20230311131147-efbdcda51732 // indirect
	github.com/takara9/marmot v0.8.6 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.etcd.io/etcd/api/v3 v3.6.1 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.1 // indirect
	go.etcd.io/etcd/client/v3 v3.6.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/grpc v1.71.1 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
	gopkg.in/freddierice/go-losetup.v1 v1.0.0-20170407175016-fc9adea44124 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	libvirt.org/go/libvirt v1.11006.0 // indirect
	marmot.io/config v0.8.6 // indirect
	marmot.io/db v0.8.6 // indirect
	marmot.io/lvm v0.8.6 // indirect
	marmot.io/virt v0.8.6 // indirect
)

replace (
	marmot.io/config => ../../pkg/config
	marmot.io/db => ../../pkg/db
	marmot.io/lvm => ../../pkg/lvm
	marmot.io/util => ../../pkg/util
	marmot.io/virt => ../../pkg/virt
)

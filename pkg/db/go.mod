module db

go 1.24.4

require (
    github.com/google/uuid v1.6.0
    github.com/onsi/ginkgo/v2 v2.26.0
    github.com/onsi/gomega v1.38.2
    github.com/takara9/lvm v0.0.0-20230311131147-efbdcda51732
    go.etcd.io/etcd/client/v3 v3.6.1
    libvirt.org/go/libvirt v1.11004.0
    marmot.io/marmot v0.8.6
 )

replace (
    marmot.io/marmot => ../marmot
)
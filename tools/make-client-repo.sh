#!/bin/bash 

echo "Creating client repository..."

TARGET_DIR="mactl"
echo "Target directory: ${TARGET_DIR}"
mkdir -p ~/${TARGET_DIR}
mkdir -p ~/${TARGET_DIR}/api
mkdir -p ~/${TARGET_DIR}/cmd
mkdir -p ~/${TARGET_DIR}/cmd/mactl
mkdir -p ~/${TARGET_DIR}/pkg
mkdir -p ~/${TARGET_DIR}/pkg/client
mkdir -p ~/${TARGET_DIR}/pkg/config
mkdir -p ~/${TARGET_DIR}/pkg/types

cp ../TAG ~/${TARGET_DIR}/TAG
cp go.mod.root ~/${TARGET_DIR}/go.mod
cp Makefile.root ~/${TARGET_DIR}/Makefile
cp go.mod.api ~/${TARGET_DIR}/api/go.mod
cp go.mod.client ~/${TARGET_DIR}/pkg/client/go.mod
cp go.mod.config ~/${TARGET_DIR}/pkg/config/go.mod
cp go.mod.types ~/${TARGET_DIR}/pkg/types/go.mod
cp Makefile.mactl ~/${TARGET_DIR}/cmd/mactl/Makefile
cp ../cmd/mactl/main.go ~/${TARGET_DIR}/cmd/mactl
cp -r ../cmd/mactl/cmd ~/${TARGET_DIR}/cmd/mactl

cp ../api/marmot-api-v1.go ~/${TARGET_DIR}/api/
cp ../pkg/config/config.go ~/${TARGET_DIR}/pkg/config
cp ../pkg/config/hypervisor_config.go ~/${TARGET_DIR}/pkg/config
cp ../pkg/client/marmot-client.go ~/${TARGET_DIR}/pkg/client
cp ../pkg/types/types.go ~/${TARGET_DIR}/pkg/types


TARGET="github.com/takara9/marmot/cmd/mactl/cmd"
REPLACE="main/cmd/mactl/cmd"
sed -i s%${TARGET}%${REPLACE}%g ~/${TARGET_DIR}/cmd/mactl/main.go

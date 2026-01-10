#!/bin/bash -e

qemu-nbd --disconnect /dev/nbd0
lvremove -y /dev/vg1/lvos_test
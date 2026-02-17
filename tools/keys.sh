#!/bin/bash

while true; do
  etcdctl get /marmot --prefix --keys-only |grep -v -e '^#' -e '^$' |sort -r > /tmp/keys.txt
  LINE_NUM=0
  while read line; do
    let LINE_NUM=LINE_NUM+1
    echo ${LINE_NUM} ":" $line
  done < /tmp/keys.txt

  while true; do
  read -p "Enter the line number to view the value: " LINE_NUM
   case $LINE_NUM in
    [0-9]* ) 
        KEY=$(sed -n "${LINE_NUM}p" /tmp/keys.txt)
        etcdctl get $KEY --print-value-only |jq -r
        ;;
    [Yy]* ) break;;
    [Nn]* ) exit;;
    * ) echo "Please answer yes or no.";;
  esac
  done

done

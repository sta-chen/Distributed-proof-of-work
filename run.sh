#!/bin/bash

set -e
make

cmds=("./tracing-server"  "./coordinator" "./worker --listen 127.0.0.1:29366 --id worker0"
  "./worker --listen 127.0.0.1:8325 --id worker1" "./worker --listen 127.0.0.1:4404 --id worker2"
  "./worker --listen 127.0.0.1:28391 --id worker3" "./worker --listen 127.0.0.1:11111 --id worker4"
  "./worker --listen 127.0.0.1:23333 --id worker5" "./worker --listen 127.0.0.1:10101 --id worker6"
  "./worker --listen 127.0.0.1:22901 --id worker7" "./client")
for cmd in "${cmds[@]}"; do {
  $cmd & pid=$!
  PID_LIST+=" $pid";
  sleep .1
} done

trap "kill $PID_LIST" SIGINT

wait $PID_LIST

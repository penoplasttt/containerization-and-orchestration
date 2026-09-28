#!/bin/bash

CGROUP="/sys/fs/cgroup/lab_container"
sudo mkdir -p $CGROUP

echo 50M | sudo tee $CGROUP/memory.max > /dev/null
echo 0 | sudo tee $CGROUP/memory.swap.max > /dev/null
echo "50000 100000" | sudo tee $CGROUP/cpu.max > /dev/null
echo 200 | sudo tee $CGROUP/pids.max > /dev/null

echo $$ | sudo tee $CGROUP/cgroup.procs > /dev/null

echo "Запуск сервиса в изолированном окружении..."

exec sudo unshare --user --map-root-user --pid --fork --mount-proc --mount --net --uts --ipc \
    capsh --drop=cap_sys_time -- -c "ip link set lo up && ./app"
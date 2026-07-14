from "registry.test.pensando.io:5000/device-metrics-exporter-build:v1.16"

user = getenv("USER")
group = getenv("GROUP_NAME")
uid = getenv("USER_UID")
gid = getenv("USER_GID")


# add amd DNS nameserver
run "echo 'nameserver 192.168.64.2' | tee -a /etc/resolv.conf > /dev/null"

run "curl -k -o /usr/bin/asset-pull http://pm.test.pensando.io/tools/asset-pull"
run "chmod +x /usr/bin/asset-pull"
run "curl -k -o /usr/bin/asset-push http://pm.test.pensando.io/tools/asset-push"
run "chmod +x /usr/bin/asset-push"
copy "asset-build/exporter-asset-push.sh", "/exporter-asset-push.sh"
run "chmod +x /exporter-asset-push.sh"

if user == "root"
  # update user .bash_profile
  run "echo 'export GOPATH=/usr' >> /root/.bash_profile"
  run "echo 'export GOBIN=/root/go/bin' >> /root/.bash_profile"
#  run "echo 'export GOFLAGS=-mod=vendor' >> /root/.bash_profile"
  run "echo 'export PATH=/usr/local/go/bin:$PATH:$GOBIN' >> /root/.bash_profile"
  run "echo 'export GOINSECURE=github.com, google.golang.org, golang.org' >> /root/.bash_profile"

  run "localedef -i en_US -f UTF-8 en_US.UTF-8"
  env GOBIN: "/#{user}/go/bin"
else
if user != ""
  # add user
  run "groupadd -g #{gid} #{group}"
  run "useradd -l -u #{uid} -g #{gid} #{user} -G docker"

  # go installs in /usr, make it world writeable
  run "chmod 777 /usr/bin"

  # update user .bash_profile
  run "echo 'export GOPATH=/usr' >> /home/#{user}/.bash_profile"
  run "echo 'export GOBIN=/home/#{user}/go/bin' >> /home/#{user}/.bash_profile"
  run "echo 'export PATH=/usr/local/go/bin:$PATH:$GOBIN' >> /home/#{user}/.bash_profile"
#  run "echo 'export GOFLAGS=-mod=vendor' >> /home/#{user}/.bash_profile"
  run "echo 'sudo chown -R #{user} /sw/' >> /home/#{user}/.bash_profile"
  run "echo 'sudo chgrp -R #{user} /sw/' >> /home/#{user}/.bash_profile"
  run "echo 'Defaults secure_path = /usr/local/go/bin:$PATH:/bin:/usr/sbin/' >> /etc/sudoers"

  env GOBIN: "/home/#{user}/go/bin"
  run "echo '#{user} ALL=(root) NOPASSWD:ALL' > /etc/sudoers.d/#{user} && chmod 0440 /etc/sudoers.d/#{user}"

  run "localedef -i en_US -f UTF-8 en_US.UTF-8"
end
end

env GOPATH: "/usr"
env GOBIN: "/usr/local/go/bin"
env GOFLAGS: "-mod=vendor"
run "git config --global --add safe.directory ${GOPATH}/src/github.com/ROCm/device-metrics-exporter"

# A scratch pad file for exporting some host/workspace particulars into container, to be used for
# recording them into build packaging.
run "echo 'HOST_HOSTNAME=#{getenv("HOST_HOSTNAME")}' >> /usr/build_host_meta_data"
run "echo 'HOST_WORKSPACE=#{getenv("HOST_WORKSPACE")}' >> /usr/build_host_meta_data"

inside "/etc" do
  run "rm -f localtime"
  run "ln -s /usr/share/zoneinfo/US/Pacific localtime"
end

workdir "/device-metrics-exporter"

copy "entrypoint.sh", "/entrypoint.sh"
run "chmod +x /entrypoint.sh"

entrypoint "/entrypoint.sh"

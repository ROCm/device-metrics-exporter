# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


Name:           amdgpu-exporter-sriov
Version:        RPM_BUILD_VERSION
Release:        RPM_RELEASE_LABEL
Summary:        AMD GPU Metrics Exporter for RHEL SRIOV
Vendor:         AMD
License:        Apache License Version 2.0
Source0:        %{name}-%{version}-%{release}.tar.gz
BuildArch:      x86_64
URL:            https://instinct.docs.amd.com/projects/device-metrics-exporter
Requires:       systemd
VCS:            tag=%{vcs_tag};sha=%{vcs_sha};

%description
%{summary}

# stop seperate debug package generation
%define debug_package %{nil}
# disable stripping of the binaries
%define __strip /bin/true
# allow missing build-ids for precompiled binaries
%global _missing_build_ids_terminate_build 0

# disable stripping of binaries by default
# this directive coupled with debug_package being {nil} as above should be working... but it is not and
# had to short _'_strip' definition as in above.
# TO_BE_FIXED: Figure a way to do this without hacking '__strip' definition.
%define __spec_install_port /usr/lib/rpm/brp-compress

# Define source and destination paths
%define SRC_DIR    ./%{name}-%{version}-%{release}/

%define DEST_BIN   /usr/local/bin/
%define DEST_SLURM /usr/local/etc/metrics/slurm/
%define DEST_SVC   /usr/lib/systemd/system/
%define DEST_LCONF /usr/local/etc/metrics/
%define DEST_CONF  /etc/metrics/
%define DEST_LIB   /usr/local/metrics/lib


%prep
%autosetup -c -n %{name}-%{version}-%{release}

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT/usr/local/bin/
mkdir -p $RPM_BUILD_ROOT/usr/lib/systemd/system/
mkdir -p $RPM_BUILD_ROOT/usr/local/etc/metrics/slurm
mkdir -p $RPM_BUILD_ROOT/etc/metrics/
mkdir -p $RPM_BUILD_ROOT/usr/local/metrics/lib


# Install binaries
install -p %{SRC_DIR}/bin/amd-metrics-exporter-sriov $RPM_BUILD_ROOT%{DEST_BIN}/amd-metrics-exporter-sriov
install -p %{SRC_DIR}/bin/gpuagent-sriov $RPM_BUILD_ROOT%{DEST_BIN}/gpuagent-sriov
install -p %{SRC_DIR}/bin/metricsclient $RPM_BUILD_ROOT%{DEST_BIN}/metricsclient
install -p %{SRC_DIR}/bin/gpuctl $RPM_BUILD_ROOT%{DEST_BIN}/gpuctl
install -p %{SRC_DIR}/bin/metrics-exporter-ts.sh $RPM_BUILD_ROOT%{DEST_BIN}/metrics-exporter-ts.sh

# Install Systemd service unit
install -p %{SRC_DIR}/debian-sriov/usr/lib/systemd/system/amd-metrics-exporter-sriov.service $RPM_BUILD_ROOT/usr/lib/systemd/system/amd-metrics-exporter-sriov.service
install -p %{SRC_DIR}/debian-sriov/usr/lib/systemd/system/gpuagent-sriov.service $RPM_BUILD_ROOT/usr/lib/systemd/system/gpuagent-sriov.service

# install Config files
install -p %{SRC_DIR}/debian-sriov/usr/local/etc/metrics/slurm/slurm-epilog.sh  $RPM_BUILD_ROOT%{DEST_SLURM}/slurm-epilog.sh
install -p %{SRC_DIR}/debian-sriov/usr/local/etc/metrics/slurm/slurm-prolog.sh  $RPM_BUILD_ROOT%{DEST_SLURM}slurm-prolog.sh
install -p %{SRC_DIR}/debian-sriov/usr/local/etc/metrics/gpuagent.conf  $RPM_BUILD_ROOT%{DEST_LCONF}/gpuagent.conf
install -p %{SRC_DIR}/bin/config.json  $RPM_BUILD_ROOT%{DEST_CONF}/config.json

# copy libraries
install -p %{SRC_DIR}/lib/* $RPM_BUILD_ROOT%{DEST_LIB}/

%files
%defattr(-,root,root, 0755)
%attr(644, root, root) %{DEST_SLURM}/slurm-epilog.sh
%attr(644, root, root) %{DEST_SLURM}/slurm-prolog.sh
%attr(644, root, root) %{DEST_SVC}/amd-metrics-exporter-sriov.service
%attr(644, root, root) %{DEST_SVC}/gpuagent-sriov.service
%attr(644, root, root) %{DEST_LCONF}/gpuagent.conf
%attr(644, root, root) %{DEST_CONF}/config.json
%attr(644, root, root) %{DEST_LIB}/*


# binaries
%attr(755, root, root) %{DEST_BIN}/amd-metrics-exporter-sriov
%attr(755, root, root) %{DEST_BIN}/gpuagent-sriov
%attr(755, root, root) %{DEST_BIN}/metricsclient
%attr(755, root, root) %{DEST_BIN}/gpuctl
%attr(755, root, root) %{DEST_BIN}/metrics-exporter-ts.sh

%license %{SRC_DIR}/LICENSE

%doc %{SRC_DIR}/README.md

%clean

%preun
systemctl stop amd-metrics-exporter-sriov.service
systemctl stop gpuagent-sriov.service
systemctl disable gpuagent-sriov.service
systemctl disable amd-metrics-exporter-sriov.service

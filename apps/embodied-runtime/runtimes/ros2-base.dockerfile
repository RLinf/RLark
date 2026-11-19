# ROS 2 Runtime base image — provides the runtime ROS 2 / control dependencies
# for ros2-controller workloads. The Go binaries (ros2-controller, rosctr)
# are injected at deploy time via initContainer / volume mount.
#
# Build:
#   make docker-ros2
#   # or
#   docker build -t $(REGISTRY)/ros2-base:latest -f runtimes/ros2-base.dockerfile .
#
# Usage in a Pod:
#   initContainer copies binaries into a shared emptyDir, then the
#   ros2-controller container runs from this image with the binaries
#   mounted at /usr/local/bin.

ARG RUNTIME_BASE_IMAGE=ros:humble-ros-base

FROM ${RUNTIME_BASE_IMAGE}

# Optional: override the Ubuntu APT mirror host (e.g.
# https://mirrors.tuna.tsinghua.edu.cn). Rewrites archive.ubuntu.com,
# # security.ubuntu.com and ports.ubuntu.com (both http and https) in
# # /etc/apt/sources.list. The ROS apt repository (packages.ros.org) is
# # left untouched. When empty, the default repositories are used.
ARG APT_MIRROR=""

RUN if [ -n "$APT_MIRROR" ]; then \
      sed -i -E \
        -e "s#https?://archive\.ubuntu\.com/ubuntu#${APT_MIRROR}/ubuntu#g" \
        -e "s#https?://security\.ubuntu\.com/ubuntu#${APT_MIRROR}/ubuntu#g" \
        -e "s#https?://ports\.ubuntu\.com/ubuntu-ports#${APT_MIRROR}/ubuntu-ports#g" \
        /etc/apt/sources.list || true; \
    fi

# System / build toolchain — compile native ROS 2 workspace packages on host.
#   g++       — GNU C++ compiler for native workspace builds.
#   git       — clone/fetch ROS 2 packages during workspace setup.
#   iproute2  — network/routing (ip, ss, tc) for host inspection & macvlan.
#   make      — GNU make; drives colcon builds.
#   python3-colcon-common-extensions — colcon build system for ROS 2.
# Native libraries — header-only deps of the control & kinematics stack.
#   libeigen3-dev — linear algebra; transitive dep of pinocchio / tf2.
#   libfmt-dev    — {fmt} formatting library.
#   libpoco-dev   — C++ foundation library used by plugin system.
# ROS 2 control & controllers — controller framework, controllers, plugin/config.
#   ros-humble-ros2-control         — controller manager framework for ROS 2.
#   ros-humble-ros2-controllers    — joint_state / effort / position controllers.
#   ros-humble-controller-manager   — controller manager node.
#   ros-humble-realtime-tools      — realtime-safe publish buffer.
#   ros-humble-pluginlib           — plugin loading for controllers.
# ROS 2 kinematics & dynamics — transforms, KDL, and rigid-body dynamics.
#   ros-humble-tf2                  — transform library core.
#   ros-humble-tf2-ros              — transform broadcaster / listener.
#   ros-humble-tf2-eigen            — Eigen <-> tf2 conversions.
#   ros-humble-robot-state-publisher — publish /tf from URDF + joint states.
#   ros-humble-joint-state-publisher — publish joint states from a URDF.
#   ros-humble-xacro                 — parametrized URDF generation.
# ROS 2 DDS — alternative RMW implementations for DDS isolation.
#   ros-humble-rmw-cyclonedds-cpp — Cyclone DDS, recommended for multi-robot.
RUN apt-get update && apt-get install -y --no-install-recommends \
      g++ \
      git \
      iproute2 \
      make \
      python3-colcon-common-extensions \
      libeigen3-dev \
      libfmt-dev \
      libpoco-dev \
      ros-humble-ros2-control \
      ros-humble-ros2-controllers \
      ros-humble-controller-manager \
      ros-humble-realtime-tools \
      ros-humble-pluginlib \
      ros-humble-tf2 \
      ros-humble-tf2-ros \
      ros-humble-tf2-eigen \
      ros-humble-robot-state-publisher \
      ros-humble-joint-state-publisher \
      ros-humble-xacro \
      ros-humble-rmw-cyclonedds-cpp \
      && rm -rf /var/lib/apt/lists/*

# Default to Cyclone DDS for better multi-robot isolation and cross-subnet
# discovery behavior.
ENV RMW_IMPLEMENTATION=rmw_cyclonedds_cpp

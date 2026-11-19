# ROS Runtime base image — provides the runtime ROS / control dependencies
# for ros-controller workloads. The Go binaries (ros-controller, rosctr)
# are injected at deploy time via initContainer / volume mount.
#
# Build:
#   make docker-ros
#   # or
#   docker build -t $(REGISTRY)/ros-base:latest -f runtimes/ros-base.dockerfile .
#
# Usage in a Pod:
#   initContainer copies binaries into a shared emptyDir, then the
#   ros-controller container runs from this image with the binaries
#   mounted at /usr/local/bin.

ARG RUNTIME_BASE_IMAGE=ros:noetic-ros-base

FROM ${RUNTIME_BASE_IMAGE}

# Optional: override the Ubuntu APT mirror host (e.g.
# https://mirrors.tuna.tsinghua.edu.cn). Rewrites archive.ubuntu.com,
# security.ubuntu.com and ports.ubuntu.com (both http and https) in
# /etc/apt/sources.list. The ROS apt repository (packages.ros.org) is
# left untouched. When empty, the default repositories are used.
ARG APT_MIRROR=""

RUN if [ -n "$APT_MIRROR" ]; then \
      sed -i -E \
        -e "s#https?://archive\.ubuntu\.com/ubuntu#${APT_MIRROR}/ubuntu#g" \
        -e "s#https?://security\.ubuntu\.com/ubuntu#${APT_MIRROR}/ubuntu#g" \
        -e "s#https?://ports\.ubuntu\.com/ubuntu-ports#${APT_MIRROR}/ubuntu-ports#g" \
        /etc/apt/sources.list; \
    fi

# System / build toolchain — compile native ROS workspace packages on host.
#   g++      — GNU C++ compiler for native workspace builds.
#   git      — clone/fetch ROS packages during workspace setup.
#   iproute2 — network/routing (ip, ss, tc) for host inspection & macvlan.
#   make     — GNU make; drives catkin / colcon builds.
# Native libraries — header-only deps of the control & kinematics stack.
#   libeigen3-dev — linear algebra; transitive dep of pinocchio / tf.
#   libfmt-dev    — {fmt} formatting library used by ros-control.
#   libpoco-dev   — C++ foundation library used by roscpp / plugin system.
# ROS control & controllers — controller framework, controllers, plugin/config.
#   ros-noetic-actionlib           — action client/server for controller goals.
#   ros-noetic-control-msgs        — trajectory / feedback messages for controllers.
#   ros-noetic-control-toolbox     — PID and sine controllers.
#   ros-noetic-dynamic-reconfigure — runtime parameter tuning.
#   ros-noetic-pluginlib           — plugin loading for controllers.
#   ros-noetic-realtime-tools      — realtime-safe publish buffer.
#   ros-noetic-ros-control         — controller manager framework.
#   ros-noetic-ros-controllers     — joint_state / effort / position controllers.
# ROS kinematics & dynamics — transforms, KDL, and rigid-body dynamics.
#   ros-noetic-boost-sml         — Boost.SML state machines for behavior logic.
#   ros-noetic-eigen-conversions — Eigen <-> ROS message conversions.
#   ros-noetic-kdl-parser        — URDF -> KDL tree.
#   ros-noetic-pinocchio         — rigid-body dynamics (RNEA / ABA / CRBA).
#   ros-noetic-tf                — transform tree library.
#   ros-noetic-tf-conversions    — KDL / Eigen <-> tf conversions.
# ROS robot description — URDF generation and joint / state publishing.
#   ros-noetic-joint-state-publisher — publish joint states from a URDF.
#   ros-noetic-robot-state-publisher — publish /tf from URDF + joint states.
#   ros-noetic-xacro                 — parametrized URDF generation.
# ROS simulation (Gazebo) — gazebo bridge, ros_control interface, plugins.
#   ros-noetic-gazebo-plugins     — sensors / actuators / cameras in Gazebo.
#   ros-noetic-gazebo-ros-control — ros_control interface for Gazebo.
#   ros-noetic-gazebo-ros-pkgs    — Gazebo <-> ROS bridge.
RUN apt-get update && apt-get install -y --no-install-recommends \
      g++ \
      git \
      iproute2 \
      make \
      libeigen3-dev \
      libfmt-dev \
      libpoco-dev \
      ros-noetic-actionlib \
      ros-noetic-control-msgs \
      ros-noetic-control-toolbox \
      ros-noetic-dynamic-reconfigure \
      ros-noetic-pluginlib \
      ros-noetic-realtime-tools \
      ros-noetic-ros-control \
      ros-noetic-ros-controllers \
      ros-noetic-boost-sml \
      ros-noetic-eigen-conversions \
      ros-noetic-kdl-parser \
      ros-noetic-pinocchio \
      ros-noetic-tf \
      ros-noetic-tf-conversions \
      ros-noetic-joint-state-publisher \
      ros-noetic-robot-state-publisher \
      ros-noetic-xacro \
      ros-noetic-gazebo-plugins \
      ros-noetic-gazebo-ros-control \
      ros-noetic-gazebo-ros-pkgs \
      && rm -rf /var/lib/apt/lists/*

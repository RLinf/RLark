# The image of the Addon s1u-robot (apps/rlark/pkg/addons/catalog/s1u-robot): the node tuner, the Orin's keeper, the
# robot's power and cameras, and the clock of every S1U robot x86. ROS noetic, as the robot's vendor software runs it,
# for the power and the cameras; what runs on the host is the host's (node_profile.sh through nsenter).
# ROS is the office registry's mirror/ by digest (astribot/release/mirror copies it there; elsewhere
# docker.io/library/ros by the same digest: --build-arg), and apt reaches Ubuntu's
# and ROS's archives at repo.huaweicloud.com over HTTPS, ROS's signed by Open Robotics' key (ros-archive-keyring.gpg,
# C1CF6E31E6BADE8868B172B4F42ED6FBAB17C654). OpenCV makes the camera wall's thumbnails.
ARG ROS_IMAGE=registry.rlark.internal:5443/mirror/ros@sha256:4c1435fd85be3edde3820f0d132ab30a6b030209dce4fa3ac428a2aa5a763caf
FROM ${ROS_IMAGE} AS image
COPY astribot/s1u/ros-archive-keyring.gpg /usr/share/keyrings/ros-archive-keyring.gpg
RUN sed -i -e 's#http://archive.ubuntu.com/ubuntu/#https://repo.huaweicloud.com/ubuntu/#' -e 's#http://security.ubuntu.com/ubuntu/#https://repo.huaweicloud.com/ubuntu/#' /etc/apt/sources.list \
    && echo 'deb [signed-by=/usr/share/keyrings/ros-archive-keyring.gpg] https://repo.huaweicloud.com/ros/ubuntu focal main' > /etc/apt/sources.list.d/ros1-latest.list \
    && rm -f /etc/apt/sources.list.d/ros1-snapshots.list \
    && apt-get update && apt-get install -y --no-install-recommends python3-opencv \
      openssh-client util-linux curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY astribot/s1u/tuner /s1u/tuner
COPY astribot/s1u/orin /s1u/orin
COPY astribot/s1u/clock /s1u/clock
COPY astribot/s1u/device /s1u/device
COPY apps/rlark/pkg/gateway/astribot_s1u_capabilities.json /s1u/device/capabilities.json
ENTRYPOINT []

# The Addon's tests, in the image its programs run in (--target test; a release's build runs them first, astribot/release/run.sh).
FROM image AS test
RUN apt-get update && apt-get install -y --no-install-recommends python3-pytest && rm -rf /var/lib/apt/lists/*
COPY astribot/s1u /src/astribot/s1u
# The Addon's catalog entry, whose Orin uplink script the tests hold orin.py to.
COPY apps/rlark/pkg/addons/catalog/s1u-robot /src/apps/rlark/pkg/addons/catalog/s1u-robot
COPY apps/rlark/pkg/gateway/astribot_s1u_capabilities.json /src/apps/rlark/pkg/gateway/astribot_s1u_capabilities.json
RUN cd /src && python3 -m pytest -q astribot/s1u/tests

FROM image

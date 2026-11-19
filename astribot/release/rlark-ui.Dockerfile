# -- Base image arguments --------------------------------------------------
ARG NODE_IMAGE=node:22.20.0-bookworm-slim
ARG BASE_IMAGE=nginx:1.29.1

# -- Build stage: the console is built from the commit, not from a host's node_modules --
FROM ${NODE_IMAGE} AS build
ARG NPM_REGISTRY=https://registry.npmjs.org/
WORKDIR /src/apps/rlark-ui
COPY apps/rlark-ui/ ./
# A context may carry a host's node_modules or dist (the legacy builder ignores rlark-ui.Dockerfile.dockerignore).
RUN rm -rf node_modules dist && npm ci --no-audit --no-fund --registry=${NPM_REGISTRY} && npm run build

# -- Runtime stage ---------------------------------------------------------
FROM ${BASE_IMAGE}

# Where /api/ goes. The default is the in-cluster Service rlarkadm creates; a compose
# deployment sets it to the gateway container (astribot/release/control-plane/rlark.compose.yaml).
ENV RLARK_GATEWAY_URL=http://rlark-gateway.rlark-system.svc:8090
COPY astribot/release/rlark-ui.nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /src/apps/rlark-ui/dist /usr/share/nginx/html
# nginx answers on port 80 (whoever runs the image maps it). No EXPOSE: BuildKit's built-in Dockerfile frontend writes
# a pointer of its own into an EXPOSE's history entry, and the same console was another image every build.

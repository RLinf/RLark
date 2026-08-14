# -- Base image arguments --------------------------------------------------
ARG BASE_IMAGE=nginx:1.29.1

# -- Runtime stage ---------------------------------------------------------
FROM ${BASE_IMAGE}

COPY apps/rlark-ui/nginx.conf /etc/nginx/conf.d/default.conf
COPY apps/rlark-ui/dist /usr/share/nginx/html

EXPOSE 80

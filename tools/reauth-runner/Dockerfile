ARG NODE_IMAGE=node:24-bookworm-slim
FROM ${NODE_IMAGE}
WORKDIR /runner
RUN apt-get update \
  && apt-get install -y --no-install-recommends python3 python3-pip \
  && python3 -m pip install --no-cache-dir --break-system-packages curl_cffi \
  && rm -rf /var/lib/apt/lists/*
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY *.mjs *.py ./
ENV BIND_HOST=0.0.0.0
USER node
EXPOSE 8189
CMD ["node", "server.mjs"]

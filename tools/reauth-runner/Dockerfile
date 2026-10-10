ARG NODE_IMAGE=node:24-bookworm-slim
FROM ${NODE_IMAGE}
WORKDIR /runner
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
ENV HOME=/tmp/reauth-home XDG_CONFIG_HOME=/tmp/reauth-home/.config XDG_CACHE_HOME=/tmp/reauth-home/.cache
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npx playwright install --with-deps chromium
RUN apt-get update && apt-get install -y --no-install-recommends xvfb xauth \
    && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /tmp/reauth-home/.config /tmp/reauth-home/.cache \
    && chown -R node:node /tmp/reauth-home
COPY *.mjs ./
ENV BIND_HOST=0.0.0.0
USER node
EXPOSE 8189
CMD ["xvfb-run", "--auto-servernum", "--server-args=-screen 0 1280x900x24", "node", "server.mjs"]

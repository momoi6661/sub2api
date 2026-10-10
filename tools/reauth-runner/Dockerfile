ARG NODE_IMAGE=node:24-bookworm-slim
FROM ${NODE_IMAGE}
WORKDIR /runner
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npx playwright install --with-deps chromium
RUN apt-get update && apt-get install -y --no-install-recommends xvfb xauth \
    && rm -rf /var/lib/apt/lists/*
COPY *.mjs ./
ENV BIND_HOST=0.0.0.0
USER node
EXPOSE 8189
CMD ["xvfb-run", "--auto-servernum", "--server-args=-screen 0 1280x900x24", "node", "server.mjs"]

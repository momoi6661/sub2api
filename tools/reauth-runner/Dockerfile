ARG NODE_IMAGE=node:24-bookworm-slim
FROM ${NODE_IMAGE}
WORKDIR /runner
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY *.mjs ./
ENV BIND_HOST=0.0.0.0
USER node
EXPOSE 8189
CMD ["node", "server.mjs"]

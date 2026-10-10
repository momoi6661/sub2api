FROM node:24-bookworm-slim
WORKDIR /runner
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npx playwright install --with-deps chromium
COPY *.mjs ./
ENV BIND_HOST=0.0.0.0
USER node
EXPOSE 8189
CMD ["node", "server.mjs"]

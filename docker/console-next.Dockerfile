# --- Build stage ---
FROM node:20-slim AS build
WORKDIR /app

# Copy only UI package manifests first for better layer cache
COPY ui/console-next/package*.json ./
RUN npm ci

# Copy the rest of the UI source and build
COPY ui/console-next ./
RUN npm run build

# --- Run stage ---
FROM node:20-slim
WORKDIR /app
ENV NODE_ENV=production
# Cloud Run provides $PORT; we’ll bind 8080 explicitly
ENV PORT=8080
EXPOSE 8080

# Bring the built app + node_modules
COPY --from=build /app ./

# Start Next on port 8080
CMD ["npx", "next", "start", "-p", "8080"]

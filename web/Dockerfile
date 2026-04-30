# Build stage
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm cache clean --force && npm install --no-audit --prefer-offline
COPY . .
RUN npm run build -- --configuration=production

# Production stage
FROM nginx:alpine
# Copy the browser build to the nginx html directory
COPY --from=build /app/dist/web/browser /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

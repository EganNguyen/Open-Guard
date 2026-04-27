# Build stage
FROM node:20-alpine AS build
WORKDIR /app
COPY web/package*.json ./
RUN npm install
COPY web/ .
RUN npm run build -- --configuration=production

# Production stage
FROM nginx:alpine
# Copy the browser build to the nginx html directory
COPY --from=build /app/dist/web/browser /usr/share/nginx/html
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

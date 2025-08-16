FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --frozen-lockfile
COPY . .
EXPOSE 3001
CMD ["npm", "start"]
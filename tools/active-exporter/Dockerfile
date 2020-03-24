FROM node:lts-stretch
WORKDIR /app
COPY package.json /app
COPY app.js /app
RUN npm install .&& npm cache clean --force
ENTRYPOINT npm run forever
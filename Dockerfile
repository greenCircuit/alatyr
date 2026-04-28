# reverse proxy container
FROM docker.io/nginx:1.29.7
COPY ./nginx.conf /etc/nginx/nginx.conf
EXPOSE 8081
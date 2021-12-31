FROM golang:latest

ENV APP_NAME event-tracker
ENV DOMAIN www.makeshift.dev
ENV USE_AUTOCERT true
ENV DB_USER user
ENV DB_PASSWORD password
ENV DB_PORT 3306
ENV DB_NAME test
ENV HTTP_PORT 80
ENV HTTPS_PORT 443
ENV GITHUB_SECRET secret

WORKDIR /go/src/${APP_NAME}
COPY . .

RUN go get -v ./...
RUN go build -v -o ${APP_NAME}

CMD sleep 10 && ./${APP_NAME} \
    --domain ${DOMAIN} \
    --use-autocert ${USE_AUTOCERT} \
    --db-user ${DB_USER} \
    --db-password ${DB_PASSWORD} \
    --db-port ${DB_PORT} \
    --db-name ${DB_NAME} \
    --http-port ${HTTP_PORT} \
    --https-port ${HTTPS_PORT} \
    --github-secret ${GITHUB_SECRET} \
    --slack-signing-secret ${SLACK_SIGNING_SECRET} \
    --slack-oauth-token ${SLACK_OAUTH_TOKEN}

EXPOSE ${HTTPS_PORT}
EXPOSE ${HTTP_PORT}

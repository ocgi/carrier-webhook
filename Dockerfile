FROM --platform=linux/amd64 centos:centos7
LABEL description="webhook"

COPY ./bin/webhook webhook
ENTRYPOINT ["/webhook"]

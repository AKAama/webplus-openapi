FROM hub.sudytech.cn/library/alpine:3.16
ARG TARGETARCH
ARG APP_NAME
ENV APP_NAME=${APP_NAME}
LABEL maintainer="fanxun <xunfan@sudytech.com>"
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add -U tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

COPY out/$APP_NAME-linux-${TARGETARCH} /app/$APP_NAME

RUN mkdir -p /app/etc
COPY etc/config/config.yaml /app/etc/config/config.yaml
WORKDIR /app
ENTRYPOINT ["sh","-c","/app/${APP_NAME} -c etc/config/config.yaml"]

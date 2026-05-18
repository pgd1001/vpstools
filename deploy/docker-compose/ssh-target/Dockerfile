FROM alpine:3.21

RUN apk add --no-cache openssh-server bash

RUN adduser -D svrtools && echo "svrtools:svrtools" | chpasswd

RUN ssh-keygen -A

RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin no/' /etc/ssh/sshd_config

EXPOSE 22

CMD ["/usr/sbin/sshd", "-D", "-e"]

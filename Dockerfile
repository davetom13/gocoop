FROM armhf/ubuntu
LABEL maintainer="francois.allais@hotmail.com"

ADD gocoop-api /usr/bin

WORKDIR /usr/bin

EXPOSE     8000
CMD        [ "/usr/bin/gocoop-api", "--logging", "info" ]

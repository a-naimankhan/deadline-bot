FROM ubuntu:latest
LABEL authors="tempadmin"

ENTRYPOINT ["top", "-b"]
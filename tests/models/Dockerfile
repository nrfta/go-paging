FROM golang:1.25

ARG SQLBOILER_VERSION=latest

WORKDIR /workspace

RUN go install github.com/aarondl/sqlboiler/v4@${SQLBOILER_VERSION}
RUN go install github.com/aarondl/sqlboiler/v4/drivers/sqlboiler-psql@${SQLBOILER_VERSION}

ENTRYPOINT ["sh", "-c"]

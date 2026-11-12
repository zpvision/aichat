FROM alpine:3.23
RUN apk add --no-cache poppler-utils
COPY docker/extract-pdf.sh /usr/local/bin/extract-pdf
RUN chmod 755 /usr/local/bin/extract-pdf
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/extract-pdf"]

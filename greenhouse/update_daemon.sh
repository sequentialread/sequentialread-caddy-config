#!/bin/bash

mkdir -p ./greenhouse/greenhouse-daemon && \
curl -sS -o "daemon.gz" "https://picopublish.sequentialread.com/files/greenhouse-daemon-alpha-rc0-315e67e-82d8-linux-arm.gz" && \
gzip --stdout --decompress  "daemon.gz" > "./greenhouse/greenhouse-daemon/greenhouse-daemon" && \
rm "daemon.gz" &&  chmod +x "./greenhouse/greenhouse-daemon/greenhouse-daemon" && \
curl -sS -o "threshold.gz" "https://picopublish.sequentialread.com/files/threshold-0.0.0-6cfcabd-f27e-linux-arm.gz" && \
gzip --stdout --decompress  "threshold.gz" > "./greenhouse/greenhouse-daemon/greenhouse-threshold" && \
rm "threshold.gz" &&  chmod +x "./greenhouse/greenhouse-daemon/greenhouse-threshold" &&  \
curl -sS -o "caddy.gz" "https://picopublish.sequentialread.com/files/caddy-v2.4.0-beta.2-forest-078f12e0-b4a8-linux-arm.gz" && \
gzip --stdout --decompress  "caddy.gz" > "./greenhouse/greenhouse-daemon/greenhouse-caddy" && \
rm "caddy.gz" &&  chmod +x "./greenhouse/greenhouse-daemon/greenhouse-caddy" &&  \
echo '{
  "admin": {
    "disabled": false,
    "listen": "unix///var/run/greenhouse-daemon-caddy-admin.sock",
    "config": {
      "persist": false
    }
  }
}' > ./greenhouse/greenhouse-daemon/caddy-config.json

chown -R 165536:165536 ./greenhouse/greenhouse-daemon/

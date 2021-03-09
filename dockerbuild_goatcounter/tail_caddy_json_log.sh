#!/bin/sh -e

logfilepath="$1"

tail -f "$logfilepath"  \
  | jq -r '"\(.request.host):\(.common_log) \"\(.request.headers.Referer[0])\" \"\(.request.headers."User-Agent"[0])\""' \
  | /app/goatcounter import -site https://goatcounter.beta.sequentialread.com -format combined-vhost -- -

  -datetime "02/Jan/2006:15:04:05 -0700"


  

      entrypoint: ["/bin/sh"]
    command: ["-c", "./goatcounter-caddy-log-adapter /caddylog/caddy-goatcounter.log | ./goatcounter import -site https://goatcounter.beta.sequentialread.com -format combined-vhost -- -"]

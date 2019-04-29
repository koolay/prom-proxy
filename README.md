http proxy for prometheus metrics api
---------

## Why?

prometheus can not scrape backend by `HAProxy` OR `HTTPS`.


## How to work

1. Start prom-proxy server, listen like 8111.

2. Config your prometheus file `prometheus.yml`.


```yaml

 - job_name: 'myapp'
    scrape_interval: 10s
    metrics_path: "/metrics"
    proxy_url: "http://prom-proxy:8111"

    static_configs:
         - targets: ['127.0.0.1:8111']
```

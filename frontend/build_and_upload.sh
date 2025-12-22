#!/bin/bash

pnpm run build --mode cloud
tar czf codenames.tar.gz -C build/ .
kubectl cp codenames.tar.gz data-shell:/pvc/caddy/www/codenames.ai/

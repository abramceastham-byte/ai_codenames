#!/bin/bash
go run github.com/bcspragu/Codenames/cmd/codenames-local \
  --words="$(go run github.com/bcspragu/Codenames/cmd/boardgen-cli)"

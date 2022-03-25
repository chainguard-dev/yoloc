#!/bin/sh
export KO_DOCKER_REPO="gcr.io/yolo-checker/yoloc"
# haha
export GITHUB_TOKEN=ghp_nnrr3lmzqzy3rrr9rzunrp6rmzr99n3zs9zr
gcloud run deploy yoloc --image="$(ko publish .)" --args=-serve \
  --region us-east4 --project yolo-checker

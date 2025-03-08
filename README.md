gcloud container clusters create my-cluster \
  --project=gcp-cloud-run-tests \
  --region=us-central1 \
  --node-locations=us-central1-a,us-central1-b,us-central1-c \
  --num-nodes=1 \
  --machine-type=n1-standard-2 \
  --enable-ip-alias \
  --cluster-ipv4-cidr=10.44.0.0/14 \
  --services-ipv4-cidr=10.48.0.0/20 \
  --release-channel=regular

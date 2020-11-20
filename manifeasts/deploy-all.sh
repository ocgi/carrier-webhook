#!/bin/bash

tmpdir=$(mktemp -d)

bash create-cert.sh --tmpdir ${tmpdir} --service carrier-webhook-service --namespace kube-system --secret carrier-wbssecret

bash patch-bundle.sh --tmpdir ${tmpdir}

kubectl apply -f ./kubernetes
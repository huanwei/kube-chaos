#!/bin/bash
kubectl delete ds chaos -n kube-system
#kubectl delete deployment test
#kubectl apply -f testpod.yaml
kubectl apply -f ../chaos-daemonset.yaml


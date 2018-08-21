#!/bin/bash
# arg 1: test pod name
# arg 2: test pod ip

echo "Kube-chaos TC ingress test">/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Loss test: Percentage 50%,Relate 25% Rate limit 100kbps ">> /tmp/test_output.txt
echo "Loss test: Percentage 50%,Relate 25% Rate limit 100kbps "
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos=100kbps,loss,50%,25% kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Duplicate test: Percentage 50% Rate 100kbps ">> /tmp/test_output.txt
echo "Duplicate test: Percentage 50% Rate 100kbps "
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos=100kbps,duplicate,50% kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Reorder test: Time 100ms Percengtage 50% Relate 25% Rate 100kbps ">> /tmp/test_output.txt
echo "Reorder test: Time 100ms Percengtage 50% Relate 25% Rate 100kbps "
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos=100kbps,delay,100ms,reorder,50%,25% kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Corrupt test: Percentage 30% Rate 100kbps ">> /tmp/test_output.txt
echo "Corrupt test: Percentage 30% Rate 100kbps "
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos=100kbps,corrupt,30% kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Delay test: Time 200ms Variation 50ms Rate 100kbps ">> /tmp/test_output.txt
echo "Delay test: Time 200ms Variation 50ms Rate 100kbps "
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos=100kbps,delay,200ms,50ms kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo "Clear ingress chaos"
kubectl annotate pod $1 kubernetes.io/clear-ingress-chaos= kubernetes.io/done-ingress-chaos=no --overwrite >> /dev/null
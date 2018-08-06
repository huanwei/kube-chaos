#!/bin/bash
# arg 1: test pod name
# arg 2: test pod ip


echo "Kube-chaos TC test">/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Delay test: {"Delay":{"Set":"yes","Time":"200ms","Variation":"50ms"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos="{\"Delay\":{\"Set\":\"yes\",\"Time\":\"200ms\",\"Variation\":\"50ms\"}}" kubernetes.io/done-chaos=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Loss test: {"Loss":{"Set":"yes","Percentage":"50%","Relate":"25%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos="{\"Loss\":{\"Set\":\"yes\",\"Percentage\":\"50%\",\"Relate\":\"25%\"}}" kubernetes.io/done-chaos=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Duplicate test: {"Duplicate":{"Set":"yes","Percentage":"50%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos="{\"Duplicate\":{\"Set\":\"yes\",\"Percentage\":\"50%\"}}" kubernetes.io/done-chaos=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Reorder test: {"Reorder":{"Set":"yes","Time":"1100ms","Percengtage":"50%","Relate":"25%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos="{\"Reorder\":{\"Set\":\"yes\",\"Time\":\"1100ms\",\"Percengtage\":\"50%\",\"Relate\":\"25%\"}}" kubernetes.io/done-chaos=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Corrupt test: {"Corrupt":{"Set":"yes","Percentage":"30%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/ingress-chaos="{\"Corrupt\":{\"Set\":\"yes\",\"Percentage\":\"30%\"}}" kubernetes.io/done-chaos=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt


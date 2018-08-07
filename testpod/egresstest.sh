#!/bin/bash
# arg 1: test pod name
# arg 2: test pod ip

echo "Kube-chaos TC egress test">/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Loss test: {"Loss":{"Set":"yes","Percentage":"50%","Relate":"25%"},"Rate":"100kbps"}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/egress-chaos="{\"Loss\":{\"Set\":\"yes\",\"Percentage\":\"50%\",\"Relate\":\"25%\"},\"Rate\":\"100kbps\"}" kubernetes.io/done-egress-chaos=no --overwrite
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Duplicate test: {"Duplicate":{"Set":"yes","Percentage":"50%"},"Rate":"100kbps"}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/egress-chaos="{\"Duplicate\":{\"Set\":\"yes\",\"Percentage\":\"50%\"},\"Rate\":\"100kbps\"}" kubernetes.io/done-egress-chaos=no --overwrite
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Reorder test: {"Reorder":{"Set":"yes","Time":"100ms","Percengtage":"50%","Relate":"25%"},"Rate":"100kbps"}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/egress-chaos="{\"Reorder\":{\"Set\":\"yes\",\"Time\":\"100ms\",\"Percengtage\":\"50%\",\"Relate\":\"25%\"},\"Rate\":\"100kbps\"}" kubernetes.io/done-egress-chaos=no --overwrite
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Corrupt test: {"Corrupt":{"Set":"yes","Percentage":"30%"},"Rate":"100kbps"}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/egress-chaos="{\"Corrupt\":{\"Set\":\"yes\",\"Percentage\":\"30%\"},\"Rate\":\"100kbps\"}" kubernetes.io/done-egress-chaos=no --overwrite
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Delay test: {"Delay":{"Set":"yes","Time":"200ms","Variation":"50ms"},"Rate":"100kbps"}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 kubernetes.io/egress-chaos="{\"Delay\":{\"Set\":\"yes\",\"Time\":\"200ms\",\"Variation\":\"50ms\"},\"Rate\":\"100kbps\"}" kubernetes.io/done-egress-chaos=no --overwrite
sleep 2
ping $2 -c 20 -i 0.01 -w 1 >>/tmp/test_output.txt
#!/bin/bash
# arg 1: test pod name
# arg 2: test pod ip


echo "Kube-chaos TC test">/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Delay test: {"Delay":{"Set":"yes","Time":"200ms","Variation":"50ms"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 TC-chaos="{\"Delay\":{\"Set\":\"yes\",\"Time\":\"200ms\",\"Variation\":\"50ms\"}}" chaos-done=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Loss test: {"Loss":{"Set":"yes","Percentage":"50%","Relate":"25%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 TC-chaos="{\"Loss\":{\"Set\":\"yes\",\"Percentage\":\"50%\",\"Relate\":\"25%\"}}" chaos-done=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Duplicate test: {"Duplicate":{"Set":"yes","Percentage":"50%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 TC-chaos="{\"Duplicate\":{\"Set\":\"yes\",\"Percentage\":\"50%\"}}" chaos-done=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Reorder test: {"Reorder":{"Set":"yes","Time":"1100ms","Percengtage":"50%","Relate":"25%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 TC-chaos="{\"Reorder\":{\"Set\":\"yes\",\"Time\":\"1100ms\",\"Percengtage\":\"50%\",\"Relate\":\"25%\"}}" chaos-done=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt

echo " ">> /tmp/test_output.txt
echo "Corrupt test: {"Corrupt":{"Set":"yes","Percentage":"30%"}}" >> /tmp/test_output.txt
echo " ">> /tmp/test_output.txt
kubectl annotate pod $1 TC-chaos="{\"Corrupt\":{\"Set\":\"yes\",\"Percentage\":\"30%\"}}" chaos-done=no --overwrite
sleep 12
ping $2 -c 10 >>/tmp/test_output.txt


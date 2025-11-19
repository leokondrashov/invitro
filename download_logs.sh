#!/bin/bash

NODE=$1
NAMESPACE=$2
POD=$3
OUTPUT_DIR=$4
START_TIME=$5

mkdir $OUTPUT_DIR -p
TMP_DIR=`mktemp -d`
scp $NODE:/var/log/pods/${NAMESPACE}_${POD}*/*/* $TMP_DIR

pushd $TMP_DIR
timestamp=${START_TIME:0:4}${START_TIME:5:2}${START_TIME:8:2}-${START_TIME:11:2}${START_TIME:14:2}${START_TIME:17:2}
for f in *.gz
do
	if [ "${f:6:15}" \< "$timestamp" ]
	then
		echo removing $f
		rm $f
	fi
done

gunzip *
popd

cat $TMP_DIR/* | awk "\$0 > \"${START_TIME}\"" | cut -d' ' -f 4- > $OUTPUT_DIR/${NAMESPACE}_${POD}_$NODE.log
rm -rf $TMP_DIR

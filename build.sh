#!/bin/bash

appName=clickvisual
Version=$1

docker build -t $appName:$Version . -f Dockerfile 

docker tag $appName:$Version registry-vpc.cn-hangzhou.aliyuncs.com/vtradex/$appName:$Version

docker push registry-vpc.cn-hangzhou.aliyuncs.com/vtradex/$appName:$Version

echo "finish vtradex/$appName:$Version"





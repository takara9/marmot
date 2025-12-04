#!/bin/bash

echo "Start JOB"
date
echo "Doing some work..."
echo "This is an error message" >&2
sleep 5
echo "Finish JOB"
date
exit 1


#!/bin/bash

COVERALLS="lAKAWPzcGsD3A8yBX3BGGtRUdJ6CaGERL"

echo "mode: count" > acc.out
for Dir in . $(find ./* -maxdepth 10 -type d ); 
do
	if ls $Dir/*.go &> /dev/null;
	then
		returnval=`go test -coverprofile=profile.out -covermode=count $Dir`
		echo ${returnval}
		if [[ ${returnval} != *FAIL* ]]
		then
    		if [ -f profile.out ]
    		then
        		cat profile.out | grep -v "mode: count" >> acc.out 
    		fi
    	else
    		exit 1
    	fi	
    fi
done

cat acc.out | go run docs/merge-coverprofile.go > merged.out

echo "REPO TOKEN: $COVERALLS"
if [ -n "$COVERALLS" ]
then
    echo "goveralls......"
    goveralls -service travis-ci -coverprofile=merged.out -repotoken $COVERALLS
fi

if [ -n "$COVERHTML" ]
then
    echo "HTML..."
    go tool cover -html=merged.out
fi	

rm -rf ./profile.out
rm -rf ./acc.out
rm -rf ./integration-acc.out
rm -rf ./merged.out

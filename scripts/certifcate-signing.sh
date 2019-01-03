#!/usr/bin/env bash
trap "exit" INT TERM ERR
trap "kill 0" EXIT
set -o errexit

. $(dirname $0)/utils.sh

user_id="user@example.com"
difficulty=16

#temp_build storagenode certificates
certificates=certificates
tmp=$(mktemp -d)
echo $tmp
#trap "rm -rf ${tmp}" EXIT

# TODO: create separate signer CA and use `--signer.ca` options
#                    --signer.ca.cert-path ${signer_cert} \
#                    --signer.ca.key-path ${signer_key} \

echo "setting up certificate signing server"
$certificates setup --overwrite \
                    --config-dir ${tmp} \
                    --signer.min-difficulty ${difficulty} \
                    #>/dev/null 2>&1

echo "creating test authorization"
$certificates auth create 1 ${user_id} >/dev/null 2>&1
token=$(certificates auth export --out - 2>&1|cut -d , -f 2|grep -oE "$user_id:\w+")

echo "starting certificate signing server"
server_out=>($certificates run --server.address 127.0.0.1:0 2>&1)
sleep 2
#echo <server_out

grep - address <server_out|sed -E 's,address":\s+"[\d+\.:],$1,'

exit 0
#wait
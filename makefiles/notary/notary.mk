.PHONY: all mysql signer server

all: mysql signer server

mysql:
	cd mysql && sudo docker build -t gravitational.com/mysql .

signer:
	sudo docker build -t gravitational.com/notary-signer -f notary-signer.dockerfile .

server:
	sudo docker build -t gravitational.com/notary-server -f notary-server.dockerfile .

push-local:
	sudo docker tag -f gravitational.com/mysql localhost:5000/notary-mysql:0.0.1
	sudo docker tag -f gravitational.com/notary-signer localhost:5000/notary-signer:0.0.3
	sudo docker tag -f gravitational.com/notary-server localhost:5000/notary-server:0.0.2

	sudo docker push localhost:5000/notary-mysql:0.0.1
	sudo docker push localhost:5000/notary-signer:0.0.3
	sudo docker push localhost:5000/notary-server:0.0.2

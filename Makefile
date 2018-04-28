include .env
export $(shell sed 's/=.*//' .env | grep -v '\#')
export VERSION=0.0.1

db-run:
	docker-compose up -d db
db-create:
	unset PGDATABASE && psql -h localhost --dbname="" -c "create database ${PGDATABASE};"
db-drop:
	PGDATABASE= psql -h localhost -c "drop database ${PGDATABASE};"
db:
	psql -h localhost 
dev:
	go run main.go
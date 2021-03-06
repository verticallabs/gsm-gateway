include .env
export $(shell sed 's/=.*//' .env | grep -v '\#')
export VERSION=0.3.0

db-run:
	docker-compose up -d db
db-create:
	docker-compose exec -e "PGUSER=${PGUSER}" -e "PGPASSWORD=${PGPASSWORD}" db psql -h 127.0.0.1 --dbname="" -c "create database ${PGDATABASE};"
db-drop:
	docker-compose exec -e "PGUSER=${PGUSER}" -e "PGPASSWORD=${PGPASSWORD}" db psql -h 127.0.0.1 --dbname="" -c "drop database ${PGDATABASE};"
db:
	docker-compose exec \
		-e "PGDATABASE=${PGDATABASE}" \
		-e "PGUSER=${PGUSER}" \
		-e "PGPASSWORD=${PGPASSWORD}" \
		db psql
build:
	go build
dev: build
	./gsm-gateway

docker-build:
	docker build -t pi:5000/gsm-gateway:${VERSION} .
docker-push: docker-build
	docker push pi:5000/gsm-gateway:${VERSION}
test:
	go test -v .
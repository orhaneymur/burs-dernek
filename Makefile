BASE  ?= http://localhost:8080
IMAGE ?= ghcr.io/lafed/burs
TAG   ?= 1.0.0

.PHONY: yardim
yardim:
	@echo "LAFED Burs Sistemi"
	@echo ""
	@echo "  make calistir    Yerelde derleyip calistirir (Go gerekir)"
	@echo "  make test        Testleri calistirir"
	@echo "  make imaj        Docker imajini olusturur ($(IMAGE):$(TAG))"
	@echo "  make gonder      Imaji kayit defterine yollar"
	@echo "  make duman-test  Calisan ortamda uctan uca testi calistirir"
	@echo "  make dev         docker compose ile yerel ortami ayaga kaldirir"
	@echo "  make dev-kapat   Yerel ortami kapatir"
	@echo "  make dagit       Kubernetes'e uygular (k8s/)"
	@echo "  make durum       Kubernetes'teki durumu gosterir"
	@echo "  make gunluk      Uygulama gunluklerini izler"
	@echo "  make yedek-al    Yedekleme isini elle tetikler"

.PHONY: calistir
calistir:
	go run ./cmd/server

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: imaj
imaj:
	docker build -t $(IMAGE):$(TAG) -t $(IMAGE):latest .

.PHONY: gonder
gonder: imaj
	docker push $(IMAGE):$(TAG)
	docker push $(IMAGE):latest

.PHONY: duman-test
duman-test:
	BASE=$(BASE) bash scripts/duman-testi.sh

.PHONY: dev
dev:
	docker compose up --build

.PHONY: dev-kapat
dev-kapat:
	docker compose down

.PHONY: dagit
dagit:
	kubectl apply -f k8s/

.PHONY: durum
durum:
	kubectl -n burs get pods,svc,ingress,pvc

.PHONY: gunluk
gunluk:
	kubectl -n burs logs -l app=burs -f --tail=100

.PHONY: yedek-al
yedek-al:
	kubectl -n burs create job --from=cronjob/burs-yedek burs-yedek-elle-$$(date +%s)

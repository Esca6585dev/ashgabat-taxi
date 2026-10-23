# Aşgabat Taxi — işe girizmek ädimleri

Ulgam üç bölekden durýar:

1. **Backend** (`backend/`, Go) — API, WebSocket, dispetçer, admin panel (`/admin`) we web sahypa (`/`).
2. **Müşderi programmasy** (`apps/client_app`, Flutter).
3. **Sürüji programmasy** (`apps/driver_app`, Flutter).

## 1. Serweri işletmek

### Docker bilen (önümçilik)

```bash
export JWT_SECRET=$(openssl rand -hex 32)
export ADMIN_PASSWORD='güýçli-parol'
# SMS (SMPP) maglumatlaryny taslamanyň kök bukjasyndaky .env faýlyna ýazyň (git-e goşulmaýar), aşakda seret
docker compose up -d --build
```

- API: `http://SERWER:8080/api/v1`, admin panel: `http://SERWER:8080/admin`
- Öňünde HTTPS üçin nginx / Caddy goýuň (programmalar `wss://` bilen birikýär).
- Admin: `ADMIN_PHONE` / `ADMIN_PASSWORD` bilen giriň we paroly **Sazlamalar** bölüminde üýtgediň.

### Docker-siz (ösüş)

```bash
cd backend && cp .env.example .env && go run ./cmd/server
```

SQLite ulanýar, hiç zat gurnamak gerek däl. `OTP_FIXED_CODE=1234` — synag üçin SMS kody.

## SMS kody (SMPP)

Müşderiler hemişe SMS kody bilen girýär. Kod operatoryň SMS merkezine SMPP arkaly iberilýär.
Taslamanyň kök bukjasynda (`docker-compose.yml` bilen ýanaşyk) `.env` faýly dörediň:

```
SMPP_IP=…
SMPP_PORT=…
SMPP_FROM=…
SMPP_USERNAME=…
SMPP_PASSWORD='…'
```

- Parolda `$` bar bolsa, ony ýeke dyrnaga alyň (`'…'`), ýogsam `$` bilen başlaýan bölek pozulýar.
- `SMPP_IP` bellense, serwer SMS-i özi SMPP arkaly iberýär. Konsolda `smpp: bound to …` ýazgysy birikmäniň üstünlikli bolandygyny görkezýär.
- Operator belgini başga görnüşde isleýän bolsa: `SMPP_DST_FORMAT=local` (65123456), `8` (865123456) ýa-da `plus` (+99365123456).
- Önümçilikde `OTP_FIXED_CODE` boş bolmaly we `APP_ENV=production` bolmaly (kod API jogabynda görkezilmeýär).
- Türkmen harply tekst (ä, ň, ý…) UCS-2 kodlamasynda iberilýär.

## Sürüjileriň paroly

Sürüji programmasyna iki ýol bilen girip bolýar: **SMS kod** ýa-da **telefon + parol**.

- Admin panel → **Sürüjiler** → «+ Sürüji hasaby (parol bilen)» — täze sürüjä telefon we parol berýärsiňiz.
- Bar bolan sürüji üçin hatardaky **Parol** düwmesi paroly täzeden belleýär (ýalňyş synanyşyk gulpuny hem aýyrýar).
- Sürüji özi hem programmada **Profil → Parol bellemek** arkaly parol goýup bilýär.
- 5 gezek ýalňyş parol girizilse, ol belgi 15 minut gulplanýar (SMS kod bilen girip bolýar).

## 2. Hakyky ýol aralyklary (OSRM, islege görä)

`OSRM_URL` boş bolsa aralyk göni çyzyk × 1.3 hasaplanýar. Takyk baha üçin:

```bash
mkdir -p osrm && cd osrm
wget https://download.geofabrik.de/asia/turkmenistan-latest.osm.pbf
docker run -t -v "$PWD:/data" osrm/osrm-backend osrm-extract -p /opt/car.lua /data/turkmenistan-latest.osm.pbf
docker run -t -v "$PWD:/data" osrm/osrm-backend osrm-partition /data/turkmenistan-latest.osrm
docker run -t -v "$PWD:/data" osrm/osrm-backend osrm-customize /data/turkmenistan-latest.osrm
```

Soňra `docker-compose.yml` içindäki `osrm` hyzmatyny açyň we `OSRM_URL=http://osrm:5000` beriň.

## 3. Programmalary gurnamak

```bash
cd apps/client_app
flutter build apk --release --dart-define=API_URL=https://api.siziň-domen.tm
cd ../driver_app
flutter build apk --release --dart-define=API_URL=https://api.siziň-domen.tm
```

Jikme-jiklikler: [`apps/README.md`](../apps/README.md).

## 4. Ilkinji gün

1. Admin panelde **Tarifler** — bahalary (başlangyç, km, minut, iň az baha, gijeki koeffisiýent) barlaň.
2. Sürüjiler sürüji programmasy arkaly hasaba durýar → admin panelde **Sürüjiler** → *Tassykla*.
3. Sürüji «Işe başla» basýar → müşderi sargyt berýär → iň ýakyn boş sürüjilere teklip gidýär.
4. Nagt syýahatda sürüji puly alýar, komissiýa onuň balansyndan aýrylýar. Balansy admin panelde
   **Sürüjiler → Balans** arkaly doldurýarsyňyz (ofisde töleg alnanda).
5. **Janly karta** — onlaýn sürüjiler we işjeň sargytlar; **Sargytlar** — sargydy el bilen sürüjä bellemek ýa-da ýatyrmak.

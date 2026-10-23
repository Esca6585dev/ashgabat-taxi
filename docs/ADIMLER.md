# Aşgabat Taxi — işe girizmek ädimleri

Ulgam üç bölekden durýar:

1. **Backend** (`backend/`, Go) — API, WebSocket, dispetçer, admin panel (`/admin`) we web sahypa (`/`).
2. **Müşderi programmasy** (`apps/client_app`, Flutter).
3. **Sürüji programmasy** (`apps/driver_app`, Flutter).

## 1. Serweri işletmek

### Docker bilen (önümçilik)

Serwerde Docker we `docker compose` gurnalan bolmaly. Domen (meselem `api.taxi.tm`) serweriň IP-sine görkezmeli,
80 we 443 portlary açyk bolmaly.

```bash
git clone https://github.com/Esca6585dev/ashgabat-taxi.git && cd ashgabat-taxi
cp .env.example .env
nano .env        # JWT_SECRET (openssl rand -hex 32), ADMIN_PASSWORD, DOMAIN, PUBLIC_URL, SMPP_…
docker compose --profile https up -d --build
docker compose logs -f api      # «listening on :8080 (production)» we «smpp: bound to …» ýazgylaryny barlaň
```

- `--profile https` HTTPS üçin Caddy-ni hem işledýär. Sertifikat (Let's Encrypt) awtomatik alynýar we täzelenýär.
  Domen ýok bolsa, `--profile https` goşmaň: API `http://SERWER_IP:8080` salgysynda işleýär.
- API: `https://DOMAIN/api/v1`, admin panel: `https://DOMAIN/admin`, web programmalar: `https://DOMAIN/app/` we `/driver/`.
- `JWT_SECRET` bellenmese ýa-da esasy baha galsa, serwer önümçilikde işe başlamaýar (howpsuzlyk üçin).
- Admin: `ADMIN_PHONE` / `ADMIN_PASSWORD` bilen giriň we paroly **Sazlamalar** bölüminde üýtgediň.
- Maglumatlar `pgdata` Docker göwrüminde saklanýar. Ätiýaçlyk nusga:
  `docker compose exec db pg_dump -U taxi taxi > backup.sql`
- Täzelemek: `git pull && docker compose --profile https up -d --build`

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

## Sürüjileriň tölegi we kart bilen balans doldurmak

### Iki töleg görnüşi

| Görnüş | Nähili işleýär |
|---|---|
| **Göterim** (`commission`) | Her syýahatdan komissiýa (`COMMISSION_RATE`, meselem 10%). Nagt syýahatda balansdan aýrylýar. Balans `DRIVER_MIN_BALANCE`-dan (meselem −50 TMT) az bolsa, sürüji sargyt alyp bilmeýär. |
| **Aýlyk** (`monthly`) | 30 günde bir gezek `MONTHLY_FEE` (ýa-da sürüjiniň öz bahasy) balansdan alynýar, syýahatlardan komissiýa alynmaýar. Möhleti gutaranda balansda pul bar bolsa özi uzalýar, ýok bolsa sürüji oflaýn edilýär we «Balansy dolduryň» habaryny alýar. |

Admin panel → **Sürüjiler → Üýtget**: töleg görnüşi, komissiýa we aýlyk töleg (0 = esasy baha).
Täze sürüjileriň görnüşi: `DEFAULT_DRIVER_PLAN`.

### Kart bilen doldurmak

Sürüji programmasy → **Balans we gazanç → Kart bilen doldur** → mukdar → bankyň kart sahypasy açylýar →
töleg geçen badyna balans dolýar (programma özi barlaýar, push hem gelýär).

- Serwer balansy diňe bankdan tölegiň ýagdaýyny özi soranyndan soň doldurýar; bir töleg iki gezek goşulmaýar.
- Sürüji bank sahypasyny ýapsa-da, serwer her minutda garaşýan tölegleri bankdan barlaýar (1 sagada çenli).
- Admin panel → **Kart tölegleri**: ähli tölegler, şu gün tölenen jemi.

Bank bilen birikdirmek (Altyn Asyr / türkmen banklarynyň e-commerce, RBS REST API):

```
PAYMENT_PROVIDER=rbs
PAYMENT_RBS_URL=https://mpi.gov.tm/payment/rest     # bank başga salgy berse, şony ýazyň
PAYMENT_RBS_USERNAME=…                               # bankyň berýän merchant logini
PAYMENT_RBS_PASSWORD='…'
PUBLIC_URL=https://api.siziň-domen.tm                # bank töleg soňy şu salga gaýtarýar
```

Bank bilen şertnama baglaşylanda merchant logini/paroly we synag (test) salgysy berilýär — ilki synag salgysy bilen barlaň.
`PAYMENT_PROVIDER=demo` diňe synag üçin: hakyky pul alynmaýar (önümçilikde ulanmaň).

## Push bildirişler (Firebase)

Programma ýapyk wagty hem telefon habar berýär:

| Kime | Haçan |
|---|---|
| Sürüji | täze sargyt (sesli, ekranda çykýar), müşderi sargydy ýatyrdy, profil tassyklandy/ret edildi |
| Müşderi | sürüji tapyldy, sürüji geldi, syýahat tamamlandy, boş sürüji tapylmady, sürüji ýatyrdy, goldaw jogap berdi |

Firebase sazlamasy bolmasa hem ähli zat işleýär — diňe push gelmeýär.

1. https://console.firebase.google.com → täze taslama.
2. Iki **Android** programma goşuň: `tm.ashgabattaxi.client_app` we `tm.ashgabattaxi.driver_app`
   (`google-services.json` gerek däl).
3. **Serwer:** Project settings → Service accounts → *Generate new private key*. Faýly serwerde
   `secrets/firebase-key.json` edip goýuň we `.env` faýlyna `FCM_CREDENTIALS_FILE=/app/secrets/firebase-key.json` ýazyň
   (Docker-siz: faýlyň ýoly). Serwer işe başlanda `push: FCM enabled for project …` ýazgysy çykýar.
4. **Programmalar:** GitHub → Settings → Secrets and variables → Actions → **Variables**:

   | Üýtgeýän | Nireden |
   |---|---|
   | `FIREBASE_API_KEY` | Project settings → General → Web API Key |
   | `FIREBASE_PROJECT_ID` | Project ID |
   | `FIREBASE_SENDER_ID` | Cloud Messaging → Sender ID |
   | `FIREBASE_CLIENT_APP_ID` | müşderi programmasynyň App ID (`1:…:android:…`) |
   | `FIREBASE_DRIVER_APP_ID` | sürüji programmasynyň App ID |

   Soňra **Actions → Build apps → Run workflow**: täze APK-lar push bilen gurulýar.
5. Telefonda programma ilkinji açylanda «bildiriş ibermäge rugsat» soraýar — rugsat beriň.

iPhone üçin goşmaça Apple Push (APNs) açary Firebase-e ýüklenmeli; web görnüşinde push ýok (WebSocket ýeterlik).

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

## 3. Programmalar: APK we web

### Taýýar faýllary almak (GitHub Actions)

`main` şahasyna `apps/` üýtgeşmesi push edilende GitHub özi gurnaýar:
GitHub → **Actions → Build apps** we **Releases** sahypasynda:

| Faýl | Näme |
|---|---|
| `ashgabat-taxi-musderi.apk` | Müşderi programmasy (Android) |
| `ashgabat-taxi-suruji.apk` | Sürüji programmasy (Android) |
| `web-musderi.zip`, `web-suruji.zip` | Web görnüşleri |

Serweriň salgysyny APK-a goşmak üçin: GitHub → Settings → Secrets and variables → Actions → **Variables** → `API_URL`
(meselem `https://api.taxi.tm`), soňra **Actions → Build apps → Run workflow**. Bellenmese hem bolýar:
programmada giriş ekranyndaky ⚙ düwmesi bilen serweriň salgysy girizilýär.

Google Play üçin öz gol açaryňyzy (keystore) **Secrets**-e goşuň: `ANDROID_KEYSTORE_BASE64`,
`ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. Bolmasa APK synag açary bilen gollanýar —
telefona göni gurnamak üçin ýeterlik.

### Web görnüşi

Serwer web programmalary özi açýar: `http://SERWER:8080/app/` (müşderi) we `http://SERWER:8080/driver/` (sürüji).
Docker bilen: `web-musderi.zip` we `web-suruji.zip` faýllaryny `docker-compose.yml` ýanyndaky `web-apps/` bukjasyna açyň:

```bash
mkdir -p web-apps && cd web-apps
unzip ../web-musderi.zip && unzip ../web-suruji.zip     # client_web/ we driver_web/ bukjalary
docker compose restart api
```

Web we serwer bir salgyda bolany üçin, web programma serweriň salgysyny özi bilýär.

### Özüňiz gurnamak

```bash
cd apps/client_app
flutter build apk --release --dart-define=API_URL=https://api.siziň-domen.tm
flutter build web --release --base-href /app/
cd ../driver_app
flutter build apk --release --dart-define=API_URL=https://api.siziň-domen.tm
flutter build web --release --base-href /driver/
```

Jikme-jiklikler: [`apps/README.md`](../apps/README.md).

## 4. Ilkinji gün

1. Admin panelde **Tarifler** — bahalary (başlangyç, km, minut, iň az baha, gijeki koeffisiýent) barlaň.
2. Sürüjiler sürüji programmasy arkaly hasaba durýar → admin panelde **Sürüjiler** → *Tassykla*.
3. Sürüji «Işe başla» basýar → müşderi sargyt berýär → iň ýakyn boş sürüjilere teklip gidýär.
4. Nagt syýahatda sürüji puly alýar, komissiýa onuň balansyndan aýrylýar. Balansy admin panelde
   **Sürüjiler → Balans** arkaly doldurýarsyňyz (ofisde töleg alnanda).
5. **Janly karta** — onlaýn sürüjiler we işjeň sargytlar; **Sargytlar** — sargydy el bilen sürüjä bellemek ýa-da ýatyrmak.

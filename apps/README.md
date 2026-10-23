# Aşgabat Taxi — mobil programmalar (Flutter)

| Bukja | Näme |
|---|---|
| `client_app/` | Müşderi (ýolagçy) programmasy — taksi çagyrmak, eltip bermek |
| `driver_app/` | Sürüji programmasy — sargyt almak, ýol, gazanç |
| `taxi_core/` | Umumy kitaphana: API, modeller, WebSocket, karta, giriş ekrany |

Iki programma hem `backend/` serwerine (`/api/v1` + `/ws`) birikýär.

## Işletmek

```bash
# 1) serwer (başga terminalda)
cd backend && cp .env.example .env && go run ./cmd/server

# 2) programma — Android emulýatorda serwer 10.0.2.2:8080 salgysynda
cd apps/client_app && flutter run
# hakyky telefonda kompýuteriň IP-si bilen:
flutter run --dart-define=API_URL=http://192.168.1.10:8080
```

Synag üçin `.env` içinde `APP_ENV=development` we `OTP_FIXED_CODE=1234` bolsa, SMS kody ekranda görkezilýär.

## Gurnamak (release)

```bash
cd apps/client_app
flutter build apk --release --dart-define=API_URL=https://api.siziň-domen.tm
flutter build appbundle --release --dart-define=API_URL=https://api.siziň-domen.tm   # Google Play
flutter build ipa --dart-define=API_URL=https://api.siziň-domen.tm                     # iOS (macOS gerek)
```

`driver_app` hem şeýle. Release üçin Android gol (keystore) sazlaň — `android/app/build.gradle.kts` faýlynda `signingConfig`.

Taýýar APK we web faýllary GitHub Actions gurnaýar (`.github/workflows/build-apps.yml`) — `docs/ADIMLER.md`, 3-nji bölüm.

Serweriň salgysyny programmanyň içinde hem üýtgedip bolýar: giriş ekranynda ⚙ düwmesi.

Sazlamalar (`--dart-define`):

| Ady | Bellik |
|---|---|
| `API_URL` | Serweriň salgysy. Bellenmedik bolsa: webde sahypanyň öz salgysy, Android-de `http://10.0.2.2:8080` |
| `TILE_URL` | Karta plitkalary. Bellenmedik bolsa `https://tile.openstreetmap.org/{z}/{x}/{y}.png` |

## Barlaglar

```bash
cd apps/taxi_core && flutter test                         # modeller, formatlar
TAXI_API_URL=http://127.0.0.1:8080 flutter test           # + işleýän serwere doly syýahat barlagy
cd apps/client_app && flutter analyze
```

`test/live_api_test.dart` hakyky serwere birikip, müşderi sargydy → sürüjä WebSocket teklibi → kabul et →
geldim → ýola çyk → tamamla → baha → gazanç ýoluny barlaýar.

## Müşderi programmasy

- Telefon + SMS kod bilen giriş
- Kartada başlangyç nokat (pin), salgy gözlegi, kartadan saýlamak, saklanan salgylar (Öý, Iş…)
- Ugur we baha: ähli tarifler boýunça baha, golaýdaky sürüjiler, gijeki tarif
- Nagt / kart, promokod, sürüjä teswir, eltip bermek üçin alyjynyň telefony
- Janly sargyt: sürüji gözlenýär → sürüji ýolda (karta üstünde hereket edýär) → garaşýar → ýolda → baha
- Sürüjä jaň, ýatyrmak (sebäbi bilen), syýahatlar taryhy, goldaw gullugy, profil

## Sürüji programmasy

- Giriş: SMS kod ýa-da telefon + parol (paroly admin berýär ýa-da sürüji profilde özi belleýär)
- Awtoulag we şahadatnama bilen hasaba durmak → admin tassyklaýança garaşmak
- Onlaýn / oflaýn, GPS ýerleşişi WebSocket arkaly (her 30 sekuntda hem täzelenýär)
- Täze sargyt kartoçkasy: baha, aralyk, 30 sekunt sanaw, kabul et / ýok
- Ädimler: Geldim → Ýola çyk → Tamamla; nawigasiýa (Google/Yandex kartalaryny açýar), müşderä jaň
- Tamamlananda nagt alynmaly mukdar we komissiýa; müşderä baha
- Balans, şu gün / hepde / aý gazanjy, hereketler, sargyt taryhy, goldaw
- Töleg görnüşi: göterim ýa-da aýlyk; balansy kart bilen doldurmak (bank sahypasy), aýlyk tölegi balansdan tölemek

Push bildirişler: Firebase sazlamalary `--dart-define=FIREBASE_API_KEY=… FIREBASE_APP_ID=… FIREBASE_SENDER_ID=… FIREBASE_PROJECT_ID=…`
bilen berilýär (CI-de GitHub Variables). Bolmasa push öçük, programma adaty işleýär. Jikme-jik: `docs/ADIMLER.md`.

## Häzirlikçe ýok (indiki ädimler)

- Sürüji programmasy ýapyk (fon) wagtynda GPS ibermek — häzir programma açyk durmaly.
- Müşderiniň syýahat üçin kart bilen onlaýn tölegi (sürüjiniň balansy kart bilen dolýar, syýahat tölegi heniz nagt / bellik).
- Köp sanly ulanyjy üçin OpenStreetMap-yň mugt plitkalary ýeterlik däl: öz plitka serweriňizi guruň we `TILE_URL` beriň.

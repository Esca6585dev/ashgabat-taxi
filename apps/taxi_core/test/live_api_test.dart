// End-to-end check of ApiClient + Realtime against a running backend.
//
//   cd backend && APP_ENV=development OTP_FIXED_CODE=1234 DB_DSN=/tmp/e2e.db go run ./cmd/server
//   cd apps/taxi_core && TAXI_API_URL=http://127.0.0.1:8080 flutter test test/live_api_test.dart
//
// Skipped when TAXI_API_URL is not set.
import 'dart:io';
import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:taxi_core/taxi_core.dart';

void main() {
  final base = Platform.environment['TAXI_API_URL'];
  final adminPhone = Platform.environment['ADMIN_PHONE'] ?? '+99360000000';
  final adminPass = Platform.environment['ADMIN_PASSWORD'] ?? 'admin12345';

  test('full ride: order -> offer over WS -> accept -> arrive -> ride -> complete -> rate', () async {
    final rnd = Random().nextInt(899999) + 100000;
    final clientPhone = '+9936${rnd.toString().padLeft(7, '0')}';
    final driverPhone = '+9936${(rnd + 1).toString().padLeft(7, '0')}';

    Future<ApiClient> login(String phone, String role) async {
      final api = ApiClient(baseUrl: base);
      final dev = await api.sendOtp(phone);
      final r = await api.verifyOtp(phone, dev ?? '1234', role: role, name: role == 'driver' ? 'Sürüji Test' : 'Müşderi Test');
      api.token = r.token;
      expect(r.user.role, role);
      return api;
    }

    final client = await login(clientPhone, 'client');
    final driver = await login(driverPhone, 'driver');

    // driver registers -> pending until an admin approves
    expect(await driver.driverMe(), isNull);
    final prof = await driver.driverRegister(
      name: 'Sürüji Test',
      carBrand: 'Toyota',
      carModel: 'Corolla',
      carColor: 'Ak',
      carPlate: 'AG ${rnd % 10000}',
      carYear: 2020,
      licenseNumber: 'TM$rnd',
      tariffCodes: ['economy', 'comfort'],
    );
    expect(prof.status, DriverStatus.pending);
    await expectLater(driver.setOnline(true, lat: 37.95, lng: 58.33), throwsA(isA<ApiException>()));

    final admin = ApiClient(baseUrl: base);
    final a = await admin.post('/auth/admin/login', {'phone': adminPhone, 'password': adminPass}) as Map;
    admin.token = a['token'] as String;
    await admin.put('/admin/drivers/${prof.id}/status', {'status': 'approved'});
    expect((await driver.driverMe())!.driver.status, DriverStatus.approved);

    // realtime for both sides
    final dRt = Realtime(() => driver.wsUri)..connect();
    final cRt = Realtime(() => client.wsUri)..connect();
    addTearDown(() {
      dRt.dispose();
      cRt.dispose();
    });
    Future<WsEvent> next(Realtime rt, String type, {bool Function(WsEvent)? where}) =>
        rt.events.firstWhere((e) => e.type == type && (where == null || where(e))).timeout(const Duration(seconds: 15));
    await Future.wait([dRt.connection.firstWhere((v) => v), cRt.connection.firstWhere((v) => v)]).timeout(const Duration(seconds: 10));

    expect(await driver.setOnline(true, lat: 37.951, lng: 58.331), isTrue);
    expect(dRt.sendLocation(37.951, 58.331, 0), isTrue);

    // client side: config, tariffs, estimate
    final cfg = await client.config();
    expect(cfg.currency, isNotEmpty);
    final pickup = const Place(name: 'Test başlangyç', lat: 37.9601, lng: 58.3261);
    const dropoff = Place(name: 'Test barmaly ýer', lat: 37.93, lng: 58.39);
    final est = await client.estimate(pickup, dropoff);
    expect(est.options, isNotEmpty);
    expect(est.distanceKm, greaterThan(1));
    final economy = est.options.firstWhere((o) => o.tariff.code == 'economy');

    final offerF = next(dRt, 'order.new');
    final order = await client.createOrder(tariffId: economy.tariff.id, pickup: pickup, dropoff: dropoff, comment: 'e2e');
    expect(order.status, OrderStatus.searching);
    expect((await client.activeOrder())!.id, order.id);

    final offer = OrderOffer.fromJson((await offerF).map);
    expect(offer.order.id, order.id);
    expect(offer.order.estimatedPrice, economy.price);

    final acceptedF = next(cRt, 'order.accepted');
    final accepted = await driver.acceptOrder(order.id);
    expect(accepted.status, OrderStatus.accepted);
    final acceptedEv = Order.fromJson((await acceptedF).map);
    expect(acceptedEv.driver!.carPlate, prof.carPlate);
    expect(acceptedEv.driver!.user!.name, 'Sürüji Test');

    final locF = next(cRt, 'driver.location', where: (e) => e.map['order_id'] == order.id);
    dRt.sendLocation(37.955, 58.328, 90);
    expect((await locF).map['lat'], 37.955);

    expect((await driver.arrived(order.id)).status, OrderStatus.arrived);
    expect((await driver.startRide(order.id)).status, OrderStatus.inProgress);
    final doneF = next(cRt, 'order.completed');
    final done = await driver.completeRide(order.id);
    expect(done.status, OrderStatus.completed);
    expect(done.finalPrice, greaterThan(0));
    expect(Order.fromJson((await doneF).map).finalPrice, done.finalPrice);

    await client.rateDriver(order.id, 5, 'Gowy');
    await driver.rateClient(order.id, 5);
    expect(await client.activeOrder(), isNull);
    expect((await client.myOrders()).first.clientRating, 5);

    final earn = await driver.earnings();
    expect(earn.today.rides, 1);
    expect(earn.balance, closeTo(-done.commission, 0.01)); // cash ride: driver owes commission
    expect(earn.transactions.first.type, 'commission');
    expect((await driver.driverOrders()).first.id, order.id);

    // second order: client cancels while searching, driver gets order.taken
    final offer2F = next(dRt, 'order.new');
    final o2 = await client.createOrder(tariffId: economy.tariff.id, pickup: pickup, dropoff: dropoff);
    await offer2F;
    final takenF = next(dRt, 'order.taken', where: (e) => e.map['order_id'] == o2.id);
    await client.cancelOrder(o2.id, 'test');
    await takenF;
    expect((await client.order(o2.id)).status, OrderStatus.cancelled);

    // saved addresses + support
    final sa = await client.addAddress('Öý', dropoff);
    expect((await client.addresses()).map((e) => e.id), contains(sa.id));
    await client.deleteAddress(sa.id);
    final t = await client.createTicket('Synag ýüztutmasy', orderId: order.id);
    expect((await client.myTickets()).first.id, t.id);

    expect(await driver.setOnline(false), isFalse);
  }, skip: base == null ? 'set TAXI_API_URL to run against a live backend' : false, timeout: const Timeout(Duration(minutes: 2)));
}

// Data classes mirroring backend/internal/models (JSON from the Go API).
// Parsing is lenient: missing fields fall back to sane defaults so an older
// app keeps working when the server adds or omits fields.

double _d(dynamic v) => v is num ? v.toDouble() : double.tryParse('$v') ?? 0;
int _i(dynamic v) => v is num ? v.toInt() : int.tryParse('$v') ?? 0;
int? _iN(dynamic v) => v == null ? null : _i(v);
String _s(dynamic v) => v == null ? '' : '$v';
bool _b(dynamic v) => v == true;
DateTime? _t(dynamic v) {
  if (v == null || v == '') return null;
  final t = DateTime.tryParse('$v');
  // Go zero time ("0001-01-01T00:00:00Z") means "not set".
  if (t == null || t.year < 1900) return null;
  return t.toLocal();
}

Map<String, dynamic>? _m(dynamic v) => v is Map ? v.cast<String, dynamic>() : null;
List<T> _list<T>(dynamic v, T Function(Map<String, dynamic>) f) =>
    v is List ? v.whereType<Map>().map((e) => f(e.cast<String, dynamic>())).toList() : <T>[];

class LatLngPoint {
  final double lat;
  final double lng;
  const LatLngPoint(this.lat, this.lng);

  factory LatLngPoint.fromJson(Map<String, dynamic> j) => LatLngPoint(_d(j['lat']), _d(j['lng']));

  bool get isZero => lat == 0 && lng == 0;
}

class User {
  final int id;
  final String phone;
  final String name;
  final String role;
  final String lang;
  final bool isBlocked;
  final double rating;
  final int ratingCount;
  final bool hasPassword;
  final DriverProfile? driver;

  const User({
    required this.id,
    required this.phone,
    required this.name,
    required this.role,
    this.lang = 'tk',
    this.isBlocked = false,
    this.rating = 5,
    this.ratingCount = 0,
    this.hasPassword = false,
    this.driver,
  });

  factory User.fromJson(Map<String, dynamic> j) => User(
        id: _i(j['id']),
        phone: _s(j['phone']),
        name: _s(j['name']),
        role: _s(j['role']),
        lang: _s(j['lang']).isEmpty ? 'tk' : _s(j['lang']),
        isBlocked: _b(j['is_blocked']),
        rating: j['rating'] == null ? 5 : _d(j['rating']),
        ratingCount: _i(j['rating_count']),
        hasPassword: _b(j['has_password']),
        driver: _m(j['driver']) == null ? null : DriverProfile.fromJson(_m(j['driver'])!),
      );

  String get displayName => name.isNotEmpty ? name : phone;
}

/// Moderation state of a driver profile.
class DriverStatus {
  static const pending = 'pending';
  static const approved = 'approved';
  static const rejected = 'rejected';
  static const blocked = 'blocked';
}

class DriverProfile {
  final int id;
  final int userId;
  final User? user;
  final String status;
  final bool isOnline;
  final double lat;
  final double lng;
  final double heading;
  final String carBrand;
  final String carModel;
  final String carColor;
  final String carPlate;
  final int carYear;
  final String licenseNumber;
  final String tariffCodes;
  final double balance;
  final double commissionRate;
  final int totalRides;
  final int? currentOrderId;
  final String rejectReason;

  const DriverProfile({
    required this.id,
    required this.userId,
    this.user,
    required this.status,
    this.isOnline = false,
    this.lat = 0,
    this.lng = 0,
    this.heading = 0,
    this.carBrand = '',
    this.carModel = '',
    this.carColor = '',
    this.carPlate = '',
    this.carYear = 0,
    this.licenseNumber = '',
    this.tariffCodes = 'economy',
    this.balance = 0,
    this.commissionRate = 0,
    this.totalRides = 0,
    this.currentOrderId,
    this.rejectReason = '',
  });

  factory DriverProfile.fromJson(Map<String, dynamic> j) => DriverProfile(
        id: _i(j['id']),
        userId: _i(j['user_id']),
        user: _m(j['user']) == null ? null : User.fromJson(_m(j['user'])!),
        status: _s(j['status']),
        isOnline: _b(j['is_online']),
        lat: _d(j['lat']),
        lng: _d(j['lng']),
        heading: _d(j['heading']),
        carBrand: _s(j['car_brand']),
        carModel: _s(j['car_model']),
        carColor: _s(j['car_color']),
        carPlate: _s(j['car_plate']),
        carYear: _i(j['car_year']),
        licenseNumber: _s(j['license_number']),
        tariffCodes: _s(j['tariff_codes']),
        balance: _d(j['balance']),
        commissionRate: _d(j['commission_rate']),
        totalRides: _i(j['total_rides']),
        currentOrderId: _iN(j['current_order_id']),
        rejectReason: _s(j['reject_reason']),
      );

  String get carTitle => [carColor, carBrand, carModel].where((s) => s.isNotEmpty).join(' ');
  List<String> get tariffList => tariffCodes.split(',').map((s) => s.trim()).where((s) => s.isNotEmpty).toList();
}

class Tariff {
  final int id;
  final String code;
  final String name;
  final String nameRu;
  final String description;
  final double baseFare;
  final double perKm;
  final double perMin;
  final double minFare;
  final int freeWaitMin;
  final double perWaitMin;
  final String icon;
  final int seats;

  const Tariff({
    required this.id,
    required this.code,
    required this.name,
    this.nameRu = '',
    this.description = '',
    this.baseFare = 0,
    this.perKm = 0,
    this.perMin = 0,
    this.minFare = 0,
    this.freeWaitMin = 0,
    this.perWaitMin = 0,
    this.icon = '',
    this.seats = 4,
  });

  factory Tariff.fromJson(Map<String, dynamic> j) => Tariff(
        id: _i(j['id']),
        code: _s(j['code']),
        name: _s(j['name']),
        nameRu: _s(j['name_ru']),
        description: _s(j['description']),
        baseFare: _d(j['base_fare']),
        perKm: _d(j['per_km']),
        perMin: _d(j['per_min']),
        minFare: _d(j['min_fare']),
        freeWaitMin: _i(j['free_wait_min']),
        perWaitMin: _d(j['per_wait_min']),
        icon: _s(j['icon']),
        seats: j['seats'] == null ? 4 : _i(j['seats']),
      );
}

/// Order lifecycle: searching -> accepted -> arrived -> in_progress -> completed | cancelled.
class OrderStatus {
  static const searching = 'searching';
  static const accepted = 'accepted';
  static const arrived = 'arrived';
  static const inProgress = 'in_progress';
  static const completed = 'completed';
  static const cancelled = 'cancelled';
  static const active = [searching, accepted, arrived, inProgress];
}

class Order {
  final int id;
  final int clientId;
  final User? client;
  final int? driverId;
  final DriverProfile? driver;
  final int tariffId;
  final Tariff? tariff;
  final String status;
  final String orderType;
  final double pickupLat;
  final double pickupLng;
  final String pickupAddress;
  final double dropoffLat;
  final double dropoffLng;
  final String dropoffAddress;
  final double distanceKm;
  final double durationMin;
  final double estimatedPrice;
  final double discount;
  final double finalPrice;
  final double commission;
  final String paymentMethod;
  final String comment;
  final String recipientPhone;
  final DateTime? createdAt;
  final DateTime? acceptedAt;
  final DateTime? arrivedAt;
  final DateTime? startedAt;
  final DateTime? completedAt;
  final DateTime? cancelledAt;
  final String cancelledBy;
  final String cancelReason;
  final int clientRating;
  final int driverRating;

  const Order({
    required this.id,
    required this.clientId,
    this.client,
    this.driverId,
    this.driver,
    required this.tariffId,
    this.tariff,
    required this.status,
    this.orderType = 'ride',
    required this.pickupLat,
    required this.pickupLng,
    this.pickupAddress = '',
    required this.dropoffLat,
    required this.dropoffLng,
    this.dropoffAddress = '',
    this.distanceKm = 0,
    this.durationMin = 0,
    this.estimatedPrice = 0,
    this.discount = 0,
    this.finalPrice = 0,
    this.commission = 0,
    this.paymentMethod = 'cash',
    this.comment = '',
    this.recipientPhone = '',
    this.createdAt,
    this.acceptedAt,
    this.arrivedAt,
    this.startedAt,
    this.completedAt,
    this.cancelledAt,
    this.cancelledBy = '',
    this.cancelReason = '',
    this.clientRating = 0,
    this.driverRating = 0,
  });

  factory Order.fromJson(Map<String, dynamic> j) => Order(
        id: _i(j['id']),
        clientId: _i(j['client_id']),
        client: _m(j['client']) == null ? null : User.fromJson(_m(j['client'])!),
        driverId: _iN(j['driver_id']),
        driver: _m(j['driver']) == null ? null : DriverProfile.fromJson(_m(j['driver'])!),
        tariffId: _i(j['tariff_id']),
        tariff: _m(j['tariff']) == null ? null : Tariff.fromJson(_m(j['tariff'])!),
        status: _s(j['status']),
        orderType: _s(j['order_type']).isEmpty ? 'ride' : _s(j['order_type']),
        pickupLat: _d(j['pickup_lat']),
        pickupLng: _d(j['pickup_lng']),
        pickupAddress: _s(j['pickup_address']),
        dropoffLat: _d(j['dropoff_lat']),
        dropoffLng: _d(j['dropoff_lng']),
        dropoffAddress: _s(j['dropoff_address']),
        distanceKm: _d(j['distance_km']),
        durationMin: _d(j['duration_min']),
        estimatedPrice: _d(j['estimated_price']),
        discount: _d(j['discount']),
        finalPrice: _d(j['final_price']),
        commission: _d(j['commission']),
        paymentMethod: _s(j['payment_method']).isEmpty ? 'cash' : _s(j['payment_method']),
        comment: _s(j['comment']),
        recipientPhone: _s(j['recipient_phone']),
        createdAt: _t(j['created_at']),
        acceptedAt: _t(j['accepted_at']),
        arrivedAt: _t(j['arrived_at']),
        startedAt: _t(j['started_at']),
        completedAt: _t(j['completed_at']),
        cancelledAt: _t(j['cancelled_at']),
        cancelledBy: _s(j['cancelled_by']),
        cancelReason: _s(j['cancel_reason']),
        clientRating: _i(j['client_rating']),
        driverRating: _i(j['driver_rating']),
      );

  bool get isActive => OrderStatus.active.contains(status);
  bool get isDelivery => orderType == 'delivery';
  LatLngPoint get pickup => LatLngPoint(pickupLat, pickupLng);
  LatLngPoint get dropoff => LatLngPoint(dropoffLat, dropoffLng);

  /// Price the client actually pays (falls back to the estimate before completion).
  double get price => finalPrice > 0 ? finalPrice : estimatedPrice - discount;
}

/// A geocoder result (Nominatim) or a point picked on the map.
class Place {
  final String name;
  final String address;
  final double lat;
  final double lng;

  const Place({required this.name, this.address = '', required this.lat, required this.lng});

  factory Place.fromJson(Map<String, dynamic> j) =>
      Place(name: _s(j['name']), address: _s(j['address']), lat: _d(j['lat']), lng: _d(j['lng']));

  LatLngPoint get point => LatLngPoint(lat, lng);
  String get title => name.isNotEmpty ? name : address;

  Map<String, dynamic> toPointJson() => {'lat': lat, 'lng': lng, 'address': title};
}

class TariffOption {
  final Tariff tariff;
  final double price;
  const TariffOption(this.tariff, this.price);

  factory TariffOption.fromJson(Map<String, dynamic> j) =>
      TariffOption(Tariff.fromJson(_m(j['tariff']) ?? const {}), _d(j['price']));
}

class Estimate {
  final double distanceKm;
  final int durationMin;
  final List<LatLngPoint> polyline;
  final bool isNight;
  final List<TariffOption> options;
  final int driversNearby;
  final int pickupEtaMin;
  final String currency;

  const Estimate({
    required this.distanceKm,
    required this.durationMin,
    required this.polyline,
    required this.isNight,
    required this.options,
    required this.driversNearby,
    required this.pickupEtaMin,
    required this.currency,
  });

  factory Estimate.fromJson(Map<String, dynamic> j) => Estimate(
        distanceKm: _d(j['distance_km']),
        durationMin: _i(j['duration_min']),
        polyline: _list(j['polyline'], LatLngPoint.fromJson),
        isNight: _b(j['is_night']),
        options: _list(j['options'], TariffOption.fromJson),
        driversNearby: _i(j['drivers_nearby']),
        pickupEtaMin: _i(j['pickup_eta_min']),
        currency: _s(j['currency']).isEmpty ? 'TMT' : _s(j['currency']),
      );
}

class NearbyCar {
  final double lat;
  final double lng;
  final double heading;
  const NearbyCar(this.lat, this.lng, this.heading);

  factory NearbyCar.fromJson(Map<String, dynamic> j) => NearbyCar(_d(j['lat']), _d(j['lng']), _d(j['heading']));
}

class SavedAddress {
  final int id;
  final String title;
  final String address;
  final double lat;
  final double lng;

  const SavedAddress({required this.id, required this.title, this.address = '', required this.lat, required this.lng});

  factory SavedAddress.fromJson(Map<String, dynamic> j) =>
      SavedAddress(id: _i(j['id']), title: _s(j['title']), address: _s(j['address']), lat: _d(j['lat']), lng: _d(j['lng']));

  Place get place => Place(name: address.isNotEmpty ? address : title, lat: lat, lng: lng);
}

class PromoResult {
  final String code;
  final double discount;
  final double percent;
  final double fixed;
  const PromoResult(this.code, this.discount, {this.percent = 0, this.fixed = 0});

  factory PromoResult.fromJson(Map<String, dynamic> j) =>
      PromoResult(_s(j['code']), _d(j['discount']), percent: _d(j['discount_percent']), fixed: _d(j['discount_fixed']));

  /// Same formula as backend services.ApplyPromo (rounded to 0.5 manat).
  double discountFor(double price) {
    var d = price * percent / 100 + fixed;
    if (d > price) d = price;
    return (d * 2).roundToDouble() / 2;
  }
}

class SupportTicket {
  final int id;
  final int? orderId;
  final String message;
  final String status;
  final String reply;
  final DateTime? createdAt;

  const SupportTicket({required this.id, this.orderId, required this.message, required this.status, this.reply = '', this.createdAt});

  factory SupportTicket.fromJson(Map<String, dynamic> j) => SupportTicket(
        id: _i(j['id']),
        orderId: _iN(j['order_id']),
        message: _s(j['message']),
        status: _s(j['status']),
        reply: _s(j['reply']),
        createdAt: _t(j['created_at']),
      );
}

class Transaction {
  final int id;
  final int? orderId;
  final double amount;
  final String type;
  final String note;
  final DateTime? createdAt;

  const Transaction({required this.id, this.orderId, required this.amount, required this.type, this.note = '', this.createdAt});

  factory Transaction.fromJson(Map<String, dynamic> j) => Transaction(
        id: _i(j['id']),
        orderId: _iN(j['order_id']),
        amount: _d(j['amount']),
        type: _s(j['type']),
        note: _s(j['note']),
        createdAt: _t(j['created_at']),
      );
}

class EarningsPeriod {
  final int rides;
  final double income;
  const EarningsPeriod(this.rides, this.income);

  factory EarningsPeriod.fromJson(Map<String, dynamic>? j) =>
      j == null ? const EarningsPeriod(0, 0) : EarningsPeriod(_i(j['rides']), _d(j['income']));
}

class Earnings {
  final EarningsPeriod today;
  final EarningsPeriod week;
  final EarningsPeriod month;
  final double balance;
  final int totalRides;
  final double rating;
  final double commissionRate;
  final List<Transaction> transactions;

  const Earnings({
    required this.today,
    required this.week,
    required this.month,
    required this.balance,
    required this.totalRides,
    required this.rating,
    required this.commissionRate,
    required this.transactions,
  });

  factory Earnings.fromJson(Map<String, dynamic> j) => Earnings(
        today: EarningsPeriod.fromJson(_m(j['today'])),
        week: EarningsPeriod.fromJson(_m(j['week'])),
        month: EarningsPeriod.fromJson(_m(j['month'])),
        balance: _d(j['balance']),
        totalRides: _i(j['total_rides']),
        rating: j['rating'] == null ? 5 : _d(j['rating']),
        commissionRate: _d(j['commission_rate']),
        transactions: _list(j['transactions'], Transaction.fromJson),
      );
}

/// An order offered to a driver (WS "order.new" or /driver/orders/available).
class OrderOffer {
  final Order order;
  final double distanceKm;
  final int etaMin;
  final DateTime receivedAt;

  OrderOffer(this.order, this.distanceKm, this.etaMin) : receivedAt = DateTime.now();

  factory OrderOffer.fromJson(Map<String, dynamic> j) =>
      OrderOffer(Order.fromJson(_m(j['order']) ?? const {}), _d(j['distance_km']), _i(j['eta_min']));
}

class AppConfig {
  final String city;
  final double cityLat;
  final double cityLng;
  final String currency;
  final String supportPhone;

  const AppConfig({
    this.city = 'Aşgabat',
    this.cityLat = 37.9601,
    this.cityLng = 58.3261,
    this.currency = 'TMT',
    this.supportPhone = '+99312000000',
  });

  factory AppConfig.fromJson(Map<String, dynamic> j) => AppConfig(
        city: _s(j['city']).isEmpty ? 'Aşgabat' : _s(j['city']),
        cityLat: j['city_lat'] == null ? 37.9601 : _d(j['city_lat']),
        cityLng: j['city_lng'] == null ? 58.3261 : _d(j['city_lng']),
        currency: _s(j['currency']).isEmpty ? 'TMT' : _s(j['currency']),
        supportPhone: _s(j['support_phone']),
      );
}

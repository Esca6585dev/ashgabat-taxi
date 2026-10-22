/* Aşgabat Taxi — admin panel (vanilla JS, no build step) */
(() => {
  const API = localStorage.getItem('apiBase') || (location.origin + '/api/v1');
  const WS_URL = API.replace(/^http/, 'ws').replace(/\/api\/v1$/, '/ws');
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const CITY = [37.9601, 58.3261];
  let token = localStorage.getItem('adminToken') || '';
  let ws, map, driverMarkers = {}, orderMarkers = {}, tariffs = [];
  const state = { page: 'dashboard', orders: { page: 1 }, drivers: { page: 1 }, clients: { page: 1 } };

  // ---------- helpers ----------
  const fmtMoney = v => (Math.round((v || 0) * 100) / 100).toLocaleString('ru-RU') + ' TMT';
  const fmtDate = s => s ? new Date(s).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—';
  const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const STATUS = { searching: 'Gözlenýär', accepted: 'Kabul edildi', arrived: 'Geldi', in_progress: 'Ýolda', completed: 'Tamamlandy', cancelled: 'Ýatyryldy', pending: 'Garaşýar', approved: 'Tassyklanan', rejected: 'Ret edilen', blocked: 'Bloklanan' };
  const stat = s => `<span class="status s-${s}">${STATUS[s] || s}</span>`;
  const toast = (msg, cls = '') => { const t = document.createElement('div'); t.className = 'toast ' + cls; t.textContent = msg; $('#toasts').appendChild(t); setTimeout(() => t.remove(), 4500); };

  async function api(path, opts = {}) {
    const r = await fetch(API + path, { ...opts, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}), ...(opts.headers || {}) }, body: opts.body ? JSON.stringify(opts.body) : undefined });
    const j = await r.json().catch(() => ({}));
    if (r.status === 401 && token) { logout(); throw new Error('Sessiýa gutardy'); }
    if (!j.success) throw new Error(j.error || 'Ýalňyşlyk');
    return j.data;
  }
  const modal = html => { $('#modalBody').innerHTML = html; $('#modal').classList.remove('hidden'); };
  const closeModal = () => $('#modal').classList.add('hidden');
  $('#modal').addEventListener('click', e => { if (e.target.id === 'modal') closeModal(); });
  const pager = (el, total, pg, limit, cb) => {
    const pages = Math.max(1, Math.ceil(total / limit));
    el.innerHTML = `<span class="muted">${total} ýazgy</span><button class="btn small" ${pg <= 1 ? 'disabled' : ''} data-p="${pg - 1}">‹</button><span>${pg} / ${pages}</span><button class="btn small" ${pg >= pages ? 'disabled' : ''} data-p="${pg + 1}">›</button>`;
    $$('button', el).forEach(b => b.onclick = () => cb(+b.dataset.p));
  };

  // ---------- auth ----------
  function showLogin() { $('#login').classList.remove('hidden'); $('#app').classList.add('hidden'); }
  function logout() { token = ''; localStorage.removeItem('adminToken'); ws && ws.close(); showLogin(); }
  $('#loginForm').onsubmit = async e => {
    e.preventDefault(); $('#loginErr').textContent = '';
    try {
      const d = await api('/auth/admin/login', { method: 'POST', body: { phone: $('#loginPhone').value, password: $('#loginPass').value } });
      token = d.token; localStorage.setItem('adminToken', token); start();
    } catch (err) { $('#loginErr').textContent = err.message; }
  };
  $('#logout').onclick = logout;

  // ---------- navigation ----------
  const titles = { dashboard: 'Dashboard', map: 'Janly karta', orders: 'Sargytlar', drivers: 'Sürüjiler', clients: 'Müşderiler', tariffs: 'Tarifler', promos: 'Promokodlar', tickets: 'Ýüztutmalar', settings: 'Sazlamalar' };
  const loaders = { dashboard: loadDashboard, map: loadMap, orders: loadOrders, drivers: loadDrivers, clients: loadClients, tariffs: loadTariffs, promos: loadPromos, tickets: loadTickets, settings: () => { $('#apiBaseShow').textContent = API; } };
  function go(page) {
    if (!loaders[page]) page = 'dashboard';
    state.page = page;
    $$('.page').forEach(p => p.classList.toggle('active', p.dataset.page === page));
    $$('.side nav a').forEach(a => a.classList.toggle('active', a.dataset.page === page));
    $('#pageTitle').textContent = titles[page];
    $('.side').classList.remove('open');
    loaders[page]().catch(e => toast(e.message, 'bad'));
  }
  window.addEventListener('hashchange', () => go(location.hash.slice(1)));
  $('#menuBtn').onclick = () => $('.side').classList.toggle('open');
  setInterval(() => $('#clock').textContent = new Date().toLocaleString('ru-RU'), 1000);

  async function start() {
    $('#login').classList.add('hidden'); $('#app').classList.remove('hidden');
    tariffs = await api('/admin/tariffs').catch(() => []);
    connectWS();
    go(location.hash.slice(1) || 'dashboard');
  }

  // ---------- websocket ----------
  function connectWS() {
    if (ws) ws.close();
    ws = new WebSocket(WS_URL + '?token=' + token);
    ws.onopen = () => $('#wsState').classList.add('on');
    ws.onclose = () => { $('#wsState').classList.remove('on'); setTimeout(() => token && connectWS(), 3000); };
    ws.onmessage = e => {
      const ev = JSON.parse(e.data); const d = ev.data || {};
      switch (ev.type) {
        case 'order.created': toast(`Täze sargyt #${d.id}: ${d.pickup_address || ''}`); if (state.page === 'orders') loadOrders(); if (state.page === 'map') refreshActive(); if (state.page === 'dashboard') loadDashboard(); break;
        case 'order.accepted': case 'order.status': case 'order.completed': case 'order.cancelled':
          if (state.page === 'orders') loadOrders(); if (state.page === 'map') refreshActive(); break;
        case 'driver.location': if (map) placeDriver(d); break;
        case 'driver.online': if (state.page === 'map') loadMap(); break;
        case 'driver.registered': toast('Täze sürüji hasaba alyndy — tassyklamaga garaşýar', 'ok'); loadBadges(); break;
        case 'ticket.new': toast('Täze ýüztutma geldi'); loadBadges(); break;
      }
    };
  }
  async function loadBadges() {
    try {
      const s = await api('/admin/stats');
      const pb = $('#pendingBadge'); pb.textContent = s.drivers_pending; pb.classList.toggle('hidden', !s.drivers_pending);
      const tb = $('#ticketBadge'); tb.textContent = s.open_tickets; tb.classList.toggle('hidden', !s.open_tickets);
    } catch { }
  }

  // ---------- dashboard ----------
  async function loadDashboard() {
    const s = await api('/admin/stats');
    loadBadgesFrom(s);
    const cards = [['Şu günki sargytlar', s.orders_today], ['Şu gün tamamlanan', s.completed_today], ['Şu günki girdeji', fmtMoney(s.revenue_today)], ['Şu günki komissiýa', fmtMoney(s.commission_today)],
      ['Işjeň sargytlar', s.active_orders], ['Onlaýn sürüjiler', s.drivers_online], ['Tassyklama garaşýan', s.drivers_pending], ['Müşderiler', s.clients], ['Sürüjiler', s.drivers], ['Jemi sargyt', s.orders_total], ['Jemi girdeji', fmtMoney(s.revenue_total)], ['Açyk ýüztutma', s.open_tickets]];
    $('#statCards').innerHTML = cards.map(([l, v]) => `<div class="card stat"><div class="l">${l}</div><div class="v">${v}</div></div>`).join('');
    chart($('#chartOrders'), s.days, 'orders'); chart($('#chartRevenue'), s.days, 'revenue');
    $('#byStatus').innerHTML = (s.by_status || []).map(x => `<span class="chip">${STATUS[x.status] || x.status}: <b>${x.count}</b></span>`).join('') || '<span class="muted">Entek sargyt ýok</span>';
  }
  function loadBadgesFrom(s) { const pb = $('#pendingBadge'); pb.textContent = s.drivers_pending; pb.classList.toggle('hidden', !s.drivers_pending); const tb = $('#ticketBadge'); tb.textContent = s.open_tickets; tb.classList.toggle('hidden', !s.open_tickets); }
  function chart(el, days, key) {
    const max = Math.max(1, ...days.map(d => d[key]));
    el.innerHTML = days.map(d => `<div class="bar" style="height:${(d[key] / max) * 100}%" title="${d.date}: ${d[key]}"><span>${d[key] ? Math.round(d[key]) : ''}</span></div>`).join('');
  }

  // ---------- live map ----------
  const carIcon = (heading = 0) => L.divIcon({ className: 'car-icon', html: `<div style="transform:rotate(${heading}deg)">🚕</div>`, iconSize: [24, 24], iconAnchor: [12, 12] });
  function placeDriver(d) {
    if (!map || !d.lat) return;
    const id = d.driver_id || d.id;
    if (d.online === false) { driverMarkers[id] && map.removeLayer(driverMarkers[id]); delete driverMarkers[id]; return; }
    if (driverMarkers[id]) { driverMarkers[id].setLatLng([d.lat, d.lng]).setIcon(carIcon(d.heading)); }
    else driverMarkers[id] = L.marker([d.lat, d.lng], { icon: carIcon(d.heading) }).addTo(map).bindPopup(`Sürüji #${id}`);
  }
  async function loadMap() {
    if (typeof L === 'undefined') {
      // CDN unreachable: keep the driver / order lists working without the map
      $('#liveMap').innerHTML = '<p class="muted" style="padding:16px">Karta ýüklenmedi (Leaflet kitaphanasy açylmady). Internedi barlaň.</p>';
    } else if (!map) {
      map = L.map('liveMap').setView(CITY, 12);
      L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 19, attribution: '© OpenStreetMap' }).addTo(map);
    }
    if (map) setTimeout(() => map.invalidateSize(), 50);
    const drivers = await api('/admin/drivers/online');
    Object.values(driverMarkers).forEach(m => map.removeLayer(m)); driverMarkers = {};
    drivers.forEach(d => { placeDriver({ driver_id: d.id, lat: d.lat, lng: d.lng, heading: d.heading }); driverMarkers[d.id] && driverMarkers[d.id].bindPopup(`<b>${esc(d.user?.name)}</b><br>${esc(d.user?.phone)}<br>${esc(d.car_brand)} ${esc(d.car_model)} ${esc(d.car_plate)}`); });
    $('#onlineCount').textContent = drivers.length;
    $('#onlineList').innerHTML = drivers.map(d => `<div class="item" data-ll="${d.lat},${d.lng}">🚕 <b>${esc(d.user?.name || '—')}</b> · ${esc(d.car_plate)}<br><span class="muted">${esc(d.car_brand)} ${esc(d.car_model)} · ${d.current_order_id ? 'sargytda #' + d.current_order_id : 'boş'}</span></div>`).join('') || '<span class="muted">Onlaýn sürüji ýok</span>';
    $$('#onlineList .item').forEach(i => i.onclick = () => map && map.setView(i.dataset.ll.split(',').map(Number), 15));
    refreshActive();
  }
  async function refreshActive() {
    const { items } = await api('/admin/orders?status=active&limit=50');
    Object.values(orderMarkers).forEach(m => map.removeLayer(m)); orderMarkers = {};
    if (map) items.forEach(o => { orderMarkers[o.id] = L.circleMarker([o.pickup_lat, o.pickup_lng], { radius: 7, color: o.status === 'searching' ? '#f59e0b' : '#3b82f6', fillOpacity: .8 }).addTo(map).bindPopup(`Sargyt #${o.id} — ${STATUS[o.status]}<br>${esc(o.pickup_address)} → ${esc(o.dropoff_address)}`); });
    $('#activeList').innerHTML = items.map(o => `<div class="item" data-ll="${o.pickup_lat},${o.pickup_lng}">#${o.id} ${stat(o.status)}<br><span class="muted">${esc(o.pickup_address || '')} → ${esc(o.dropoff_address || '')}</span><br>${fmtMoney(o.final_price)} · ${esc(o.client?.phone || '')}</div>`).join('') || '<span class="muted">Işjeň sargyt ýok</span>';
    $$('#activeList .item').forEach(i => i.onclick = () => map && map.setView(i.dataset.ll.split(',').map(Number), 15));
  }

  // ---------- orders ----------
  async function loadOrders(pg) {
    if (pg) state.orders.page = pg;
    const limit = 25;
    const { items, total } = await api(`/admin/orders?status=${$('#orderStatus').value}&q=${encodeURIComponent($('#orderQ').value)}&page=${state.orders.page}&limit=${limit}`);
    $('#ordersTable tbody').innerHTML = items.map(o => `<tr>
      <td>#${o.id}</td><td>${fmtDate(o.created_at)}</td>
      <td>${esc(o.client?.name || '—')}<br><span class="muted">${esc(o.client?.phone || '')}</span></td>
      <td>${o.driver ? `${esc(o.driver.user?.name || '')}<br><span class="muted">${esc(o.driver.car_plate)}</span>` : '<span class="muted">—</span>'}</td>
      <td>${esc(o.tariff?.name || '')}${o.order_type === 'delivery' ? ' 📦' : ''}</td>
      <td>${esc(o.pickup_address || '')}<br>→ ${esc(o.dropoff_address || '')}</td>
      <td>${(o.distance_km || 0).toFixed(1)}</td>
      <td>${fmtMoney(o.final_price)}${o.discount ? `<br><span class="muted">-${o.discount}</span>` : ''}<br><span class="muted">${o.payment_method === 'card' ? 'Kart' : 'Nagt'}</span></td>
      <td>${stat(o.status)}${o.cancel_reason ? `<br><span class="muted">${esc(o.cancel_reason)}</span>` : ''}${o.client_rating ? `<br>★ ${o.client_rating}` : ''}</td>
      <td class="btns">${o.status === 'searching' ? `<button class="btn small" data-assign="${o.id}">Bellemek</button>` : ''}${['searching', 'accepted', 'arrived', 'in_progress'].includes(o.status) ? `<button class="btn small bad" data-cancel="${o.id}">Ýatyr</button>` : ''}</td>
    </tr>`).join('') || '<tr><td colspan="10" class="muted">Sargyt ýok</td></tr>';
    pager($('#ordersPager'), total, state.orders.page, limit, loadOrders);
    $$('[data-cancel]').forEach(b => b.onclick = async () => { const reason = prompt('Ýatyrmagyň sebäbi:', 'admin'); if (reason === null) return; await api(`/admin/orders/${b.dataset.cancel}/cancel`, { method: 'POST', body: { reason } }); toast('Sargyt ýatyryldy', 'ok'); loadOrders(); });
    $$('[data-assign]').forEach(b => b.onclick = async () => {
      const drivers = await api('/admin/drivers/online');
      const free = drivers.filter(d => !d.current_order_id && d.status === 'approved');
      modal(`<h3>Sargyt #${b.dataset.assign} — sürüji bellemek</h3>${free.length ? `<label>Sürüji<select id="assignSel">${free.map(d => `<option value="${d.id}">${esc(d.user?.name)} · ${esc(d.car_plate)} · ${esc(d.car_brand)}</option>`).join('')}</select></label><div class="btns"><button class="btn primary" id="assignOk">Bellemek</button><button class="btn" id="assignNo">Ýap</button></div>` : '<p class="muted">Boş onlaýn sürüji ýok</p><button class="btn" id="assignNo">Ýap</button>'}`);
      $('#assignNo').onclick = closeModal;
      $('#assignOk') && ($('#assignOk').onclick = async () => { try { await api(`/admin/orders/${b.dataset.assign}/assign`, { method: 'POST', body: { driver_id: +$('#assignSel').value } }); toast('Sürüji bellendi', 'ok'); closeModal(); loadOrders(); } catch (e) { toast(e.message, 'bad'); } });
    });
  }
  $('#orderRefresh').onclick = () => loadOrders(1); $('#orderStatus').onchange = () => loadOrders(1); $('#orderQ').onkeydown = e => e.key === 'Enter' && loadOrders(1);

  // ---------- drivers ----------
  async function loadDrivers(pg) {
    if (pg) state.drivers.page = pg;
    const limit = 25;
    const { items, total } = await api(`/admin/drivers?status=${$('#driverStatus').value}&q=${encodeURIComponent($('#driverQ').value)}&page=${state.drivers.page}&limit=${limit}`);
    $('#driversTable tbody').innerHTML = items.map(d => `<tr>
      <td>#${d.id}</td><td>${esc(d.user?.name || '—')}${d.is_online ? ' <span class="dot on"></span>' : ''}</td><td>${esc(d.user?.phone)}</td>
      <td>${esc(d.car_brand)} ${esc(d.car_model)}<br><span class="muted">${esc(d.car_color)} · ${esc(d.car_plate)} · ${d.car_year || ''}</span><br><span class="muted">Şah.: ${esc(d.license_number)}</span></td>
      <td>${esc(d.tariff_codes)}</td><td>★ ${(d.user?.rating || 5).toFixed(1)} <span class="muted">(${d.user?.rating_count || 0})</span></td><td>${d.total_rides}</td>
      <td style="color:${d.balance < 0 ? 'var(--bad)' : 'var(--ok)'}">${fmtMoney(d.balance)}</td><td>${Math.round((d.commission_rate || 0) * 100)}%</td>
      <td>${stat(d.status)}${d.reject_reason ? `<br><span class="muted">${esc(d.reject_reason)}</span>` : ''}</td>
      <td class="btns">
        ${d.status !== 'approved' ? `<button class="btn small ok" data-st="approved" data-id="${d.id}">Tassykla</button>` : ''}
        ${d.status === 'pending' ? `<button class="btn small bad" data-st="rejected" data-id="${d.id}">Ret et</button>` : ''}
        ${d.status !== 'blocked' ? `<button class="btn small" data-st="blocked" data-id="${d.id}">Blokla</button>` : ''}
        <button class="btn small" data-edit="${d.id}">Üýtget</button>
        <button class="btn small" data-bal="${d.id}">Balans</button>
      </td></tr>`).join('') || '<tr><td colspan="11" class="muted">Sürüji ýok</td></tr>';
    pager($('#driversPager'), total, state.drivers.page, limit, loadDrivers);
    $$('[data-st]').forEach(b => b.onclick = async () => {
      let reason = '';
      if (b.dataset.st !== 'approved') { reason = prompt('Sebäbi:', ''); if (reason === null) return; }
      await api(`/admin/drivers/${b.dataset.id}/status`, { method: 'PUT', body: { status: b.dataset.st, reason } }); toast('Ýagdaý üýtgedildi', 'ok'); loadDrivers(); loadBadges();
    });
    $$('[data-edit]').forEach(b => b.onclick = () => {
      const d = items.find(x => x.id == b.dataset.edit);
      modal(`<h3>Sürüji #${d.id} — ${esc(d.user?.name)}</h3>
        <label>Komissiýa (0.10 = 10%)<input id="edCom" type="number" step="0.01" min="0" max="1" value="${d.commission_rate}"></label>
        <label>Tarifler (vergul bilen)<input id="edTar" value="${esc(d.tariff_codes)}" placeholder="economy,comfort"></label>
        <p class="muted">Elýeterli: ${tariffs.map(t => t.code).join(', ')}</p>
        <div class="btns"><button class="btn primary" id="edOk">Ýatda sakla</button><button class="btn" id="edNo">Ýap</button></div>`);
      $('#edNo').onclick = closeModal;
      $('#edOk').onclick = async () => { await api(`/admin/drivers/${d.id}/status`, { method: 'PUT', body: { commission_rate: +$('#edCom').value, tariff_codes: $('#edTar').value } }); closeModal(); toast('Ýatda saklandy', 'ok'); loadDrivers(); };
    });
    $$('[data-bal]').forEach(b => b.onclick = () => {
      modal(`<h3>Balans üýtgetmek — sürüji #${b.dataset.bal}</h3>
        <label>Mukdar (TMT, + goşmak / − aýyrmak)<input id="balAmt" type="number" step="0.5" placeholder="50"></label>
        <label>Bellik<input id="balNote" placeholder="Kassa arkaly töleg"></label>
        <div class="btns"><button class="btn primary" id="balOk">Ýerine ýetir</button><button class="btn" id="balNo">Ýap</button></div>`);
      $('#balNo').onclick = closeModal;
      $('#balOk').onclick = async () => { try { await api(`/admin/drivers/${b.dataset.bal}/balance`, { method: 'POST', body: { amount: +$('#balAmt').value, note: $('#balNote').value } }); closeModal(); toast('Balans üýtgedildi', 'ok'); loadDrivers(); } catch (e) { toast(e.message, 'bad'); } };
    });
  }
  $('#driverRefresh').onclick = () => loadDrivers(1); $('#driverStatus').onchange = () => loadDrivers(1); $('#driverQ').onkeydown = e => e.key === 'Enter' && loadDrivers(1);

  // ---------- clients ----------
  async function loadClients(pg) {
    if (pg) state.clients.page = pg;
    const limit = 25;
    const { items, total } = await api(`/admin/users?q=${encodeURIComponent($('#clientQ').value)}&page=${state.clients.page}&limit=${limit}`);
    $('#clientsTable tbody').innerHTML = items.map(u => `<tr>
      <td>#${u.id}</td><td>${esc(u.name || '—')}</td><td>${esc(u.phone)}</td><td>${u.role}</td><td>★ ${(u.rating || 5).toFixed(1)} <span class="muted">(${u.rating_count})</span></td><td>${fmtDate(u.created_at)}</td>
      <td>${u.is_blocked ? stat('blocked') : '<span class="status s-approved">Işjeň</span>'}</td>
      <td>${u.role !== 'admin' ? `<button class="btn small ${u.is_blocked ? 'ok' : 'bad'}" data-block="${u.id}" data-v="${!u.is_blocked}">${u.is_blocked ? 'Aç' : 'Blokla'}</button>` : ''}</td></tr>`).join('');
    pager($('#clientsPager'), total, state.clients.page, limit, loadClients);
    $$('[data-block]').forEach(b => b.onclick = async () => { await api(`/admin/users/${b.dataset.block}/block`, { method: 'PUT', body: { blocked: b.dataset.v === 'true' } }); loadClients(); });
  }
  $('#clientRefresh').onclick = () => loadClients(1); $('#clientQ').onkeydown = e => e.key === 'Enter' && loadClients(1);

  // ---------- tariffs ----------
  const tariffForm = (t = {}) => `<h3>${t.id ? 'Tarif #' + t.id : 'Täze tarif'}</h3>
    <div class="row2"><label>Kod<input id="tCode" value="${esc(t.code || '')}" placeholder="economy"></label><label>Ady<input id="tName" value="${esc(t.name || '')}"></label></div>
    <div class="row2"><label>Ady (ru)<input id="tNameRu" value="${esc(t.name_ru || '')}"></label><label>Ikonka<input id="tIcon" value="${esc(t.icon || '')}" placeholder="economy|comfort|business|minivan|delivery"></label></div>
    <label>Düşündiriş<input id="tDesc" value="${esc(t.description || '')}"></label>
    <div class="row2"><label>Başlangyç (TMT)<input id="tBase" type="number" step="0.5" value="${t.base_fare ?? 5}"></label><label>1 km (TMT)<input id="tKm" type="number" step="0.1" value="${t.per_km ?? 1.5}"></label></div>
    <div class="row2"><label>1 min (TMT)<input id="tMin" type="number" step="0.05" value="${t.per_min ?? 0.2}"></label><label>Iň az baha<input id="tMinFare" type="number" step="0.5" value="${t.min_fare ?? 7}"></label></div>
    <div class="row2"><label>Mugt garaşma (min)<input id="tFreeWait" type="number" value="${t.free_wait_min ?? 3}"></label><label>Garaşma 1 min (TMT)<input id="tWait" type="number" step="0.1" value="${t.per_wait_min ?? 0.5}"></label></div>
    <div class="row2"><label>Koeffisiýent (surge)<input id="tMul" type="number" step="0.1" value="${t.multiplier ?? 1}"></label><label>Gije koeff. (22–06)<input id="tNight" type="number" step="0.1" value="${t.night_multiplier ?? 1.2}"></label></div>
    <div class="row2"><label>Orun sany<input id="tSeats" type="number" value="${t.seats ?? 4}"></label><label>Tertip<input id="tSort" type="number" value="${t.sort_order ?? 1}"></label></div>
    <label><span><input id="tActive" type="checkbox" style="width:auto" ${t.is_active !== false ? 'checked' : ''}> Işjeň</span></label>
    <div class="btns"><button class="btn primary" id="tOk">Ýatda sakla</button><button class="btn" id="tNo">Ýap</button></div>`;
  function openTariff(t) {
    modal(tariffForm(t)); $('#tNo').onclick = closeModal;
    $('#tOk').onclick = async () => {
      const body = { code: $('#tCode').value.trim(), name: $('#tName').value, name_ru: $('#tNameRu').value, icon: $('#tIcon').value, description: $('#tDesc').value, base_fare: +$('#tBase').value, per_km: +$('#tKm').value, per_min: +$('#tMin').value, min_fare: +$('#tMinFare').value, free_wait_min: +$('#tFreeWait').value, per_wait_min: +$('#tWait').value, multiplier: +$('#tMul').value, night_multiplier: +$('#tNight').value, seats: +$('#tSeats').value, sort_order: +$('#tSort').value, is_active: $('#tActive').checked };
      try { await api(t?.id ? `/admin/tariffs/${t.id}` : '/admin/tariffs', { method: t?.id ? 'PUT' : 'POST', body }); closeModal(); toast('Tarif ýatda saklandy', 'ok'); loadTariffs(); } catch (e) { toast(e.message, 'bad'); }
    };
  }
  async function loadTariffs() {
    tariffs = await api('/admin/tariffs');
    $('#tariffList').innerHTML = tariffs.map(t => `<div class="card ${t.is_active ? '' : 'muted'}">
      <h3>${esc(t.name)} <span class="pill">${esc(t.code)}</span>${t.is_active ? '' : ' <span class="status s-cancelled">öçürilen</span>'}</h3>
      <p class="muted">${esc(t.description)}</p>
      <p>Başlangyç <b>${t.base_fare}</b> · km <b>${t.per_km}</b> · min <b>${t.per_min}</b> · iň az <b>${t.min_fare}</b> TMT</p>
      <p class="muted">Garaşma: ${t.free_wait_min} min mugt, soň ${t.per_wait_min}/min · koeff ×${t.multiplier} · gije ×${t.night_multiplier} · ${t.seats} orun</p>
      <div class="btns"><button class="btn small" data-tedit="${t.id}">Üýtget</button>${t.is_active ? `<button class="btn small bad" data-tdel="${t.id}">Öçür</button>` : ''}</div></div>`).join('');
    $$('[data-tedit]').forEach(b => b.onclick = () => openTariff(tariffs.find(t => t.id == b.dataset.tedit)));
    $$('[data-tdel]').forEach(b => b.onclick = async () => { if (confirm('Tarifi öçürmeli?')) { await api(`/admin/tariffs/${b.dataset.tdel}`, { method: 'DELETE' }); loadTariffs(); } });
  }
  $('#tariffAdd').onclick = () => openTariff(null);

  // ---------- promos ----------
  function openPromo(p = {}) {
    modal(`<h3>${p.id ? 'Promokod' : 'Täze promokod'}</h3>
      <label>Kod<input id="pCode" value="${esc(p.code || '')}" placeholder="SALAM10"></label>
      <div class="row2"><label>Arzanladyş %<input id="pPct" type="number" value="${p.discount_percent ?? 10}"></label><label>Arzanladyş TMT<input id="pFix" type="number" step="0.5" value="${p.discount_fixed ?? 0}"></label></div>
      <div class="row2"><label>Iň köp ulanyş (0 = çäksiz)<input id="pMax" type="number" value="${p.max_uses ?? 0}"></label><label>Möhleti<input id="pExp" type="date" value="${p.expires_at ? p.expires_at.slice(0, 10) : ''}"></label></div>
      <label><span><input id="pActive" type="checkbox" style="width:auto" ${p.is_active !== false ? 'checked' : ''}> Işjeň</span></label>
      <div class="btns"><button class="btn primary" id="pOk">Ýatda sakla</button><button class="btn" id="pNo">Ýap</button></div>`);
    $('#pNo').onclick = closeModal;
    $('#pOk').onclick = async () => {
      const exp = $('#pExp').value ? new Date($('#pExp').value + 'T23:59:59').toISOString() : null;
      const body = { code: $('#pCode').value, discount_percent: +$('#pPct').value, discount_fixed: +$('#pFix').value, max_uses: +$('#pMax').value, expires_at: exp, is_active: $('#pActive').checked };
      try { await api(p.id ? `/admin/promos/${p.id}` : '/admin/promos', { method: p.id ? 'PUT' : 'POST', body }); closeModal(); loadPromos(); } catch (e) { toast(e.message, 'bad'); }
    };
  }
  async function loadPromos() {
    const ps = await api('/admin/promos');
    $('#promosTable tbody').innerHTML = ps.map(p => `<tr><td><b>${esc(p.code)}</b></td><td>${p.discount_percent}</td><td>${p.discount_fixed}</td><td>${p.uses}${p.max_uses ? ' / ' + p.max_uses : ''}</td><td>${p.expires_at ? p.expires_at.slice(0, 10) : '—'}</td><td>${p.is_active ? '✅' : '—'}</td>
      <td class="btns"><button class="btn small" data-pedit="${p.id}">Üýtget</button><button class="btn small bad" data-pdel="${p.id}">Poz</button></td></tr>`).join('') || '<tr><td colspan="7" class="muted">Promokod ýok</td></tr>';
    $$('[data-pedit]').forEach(b => b.onclick = () => openPromo(ps.find(p => p.id == b.dataset.pedit)));
    $$('[data-pdel]').forEach(b => b.onclick = async () => { if (confirm('Pozmaly?')) { await api(`/admin/promos/${b.dataset.pdel}`, { method: 'DELETE' }); loadPromos(); } });
  }
  $('#promoAdd').onclick = () => openPromo();

  // ---------- tickets ----------
  async function loadTickets() {
    const { items } = await api(`/admin/tickets?status=${$('#ticketStatus').value}&limit=100`);
    $('#ticketList').innerHTML = items.map(t => `<div class="card">
      <h3>#${t.id} · ${esc(t.user?.name || '')} <span class="muted">${esc(t.user?.phone || '')}</span> ${t.status === 'open' ? '<span class="status s-searching">açyk</span>' : '<span class="status s-completed">çözülen</span>'}</h3>
      <p>${esc(t.message)}</p>${t.order_id ? `<p class="muted">Sargyt #${t.order_id}</p>` : ''}
      ${t.reply ? `<p><b>Jogap:</b> ${esc(t.reply)}</p>` : `<textarea id="rep${t.id}" rows="2" placeholder="Jogap ýazyň..."></textarea><div class="btns" style="margin-top:8px"><button class="btn primary small" data-reply="${t.id}">Jogap ber we ýap</button></div>`}
      <p class="muted">${fmtDate(t.created_at)}</p></div>`).join('') || '<p class="muted">Ýüztutma ýok</p>';
    $$('[data-reply]').forEach(b => b.onclick = async () => { await api(`/admin/tickets/${b.dataset.reply}`, { method: 'PUT', body: { reply: $('#rep' + b.dataset.reply).value, status: 'resolved' } }); toast('Jogap iberildi', 'ok'); loadTickets(); loadBadges(); });
  }
  $('#ticketRefresh').onclick = loadTickets; $('#ticketStatus').onchange = loadTickets;

  // ---------- settings ----------
  $('#passForm').onsubmit = async e => { e.preventDefault(); try { await api('/admin/password', { method: 'POST', body: { old_password: $('#oldPass').value, new_password: $('#newPass').value } }); toast('Parol üýtgedildi', 'ok'); e.target.reset(); } catch (err) { toast(err.message, 'bad'); } };

  // ---------- boot ----------
  if (token) start().catch(showLogin); else showLogin();
})();

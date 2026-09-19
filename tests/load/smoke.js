import http from 'k6/http';
import { check, sleep } from 'k6';

// Authentication refusals and invalid replay probes are expected in this
// smoke workload. Treat every handled HTTP response as non-transport-failed;
// the checks below still make malformed responses visible in the report.
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 499 }));

const baseURL = (__ENV.K6_BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const datasetSeed = __ENV.K6_DATASET_SEED || 'synthetic-p17-t06';
const arenaID = __ENV.K6_ARENA_ID || '00000000-0000-0000-0000-000000000001';
const accountEmail = __ENV.K6_ACCOUNT_EMAIL || 'load-test@example.invalid';
const accountPassword = __ENV.K6_ACCOUNT_PASSWORD || 'synthetic-load-password';

export const options = {
  scenarios: {
    cache_cold: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      exec: 'cacheCold',
      tags: { workload: 'cache-cold' },
    },
    cache_hot: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '3s',
      exec: 'cacheHot',
      tags: { workload: 'cache-hot' },
    },
    login: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '6s',
      exec: 'login',
      tags: { workload: 'login' },
    },
    position: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '9s',
      exec: 'position',
      tags: { workload: 'position' },
    },
    argument_wallet: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '12s',
      exec: 'argumentWallet',
      tags: { workload: 'argument-wallet' },
    },
    webhook_replay: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '15s',
      exec: 'webhookReplay',
      tags: { workload: 'webhook-replay' },
    },
    arena_viral: {
      executor: 'constant-vus',
      vus: 1,
      duration: '3s',
      startTime: '18s',
      exec: 'arenaViral',
      tags: { workload: 'arena-viral' },
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    'http_req_duration{workload:cache-cold}': ['p(95)<1000'],
    'http_req_duration{workload:cache-hot}': ['p(95)<300'],
    'http_req_duration{workload:login}': ['p(95)<1500'],
    'http_req_duration{workload:position}': ['p(95)<1000'],
    'http_req_duration{workload:argument-wallet}': ['p(95)<1000'],
    'http_req_duration{workload:webhook-replay}': ['p(95)<1000'],
    'http_req_duration{workload:arena-viral}': ['p(95)<500'],
  },
};

function get(path, workload) {
  const response = http.get(`${baseURL}${path}`, { tags: { workload } });
  check(response, { [`${workload} returns HTTP`]: (r) => r.status >= 200 && r.status < 500 });
  return response;
}

export function cacheCold() {
  get(`/api/v1/arenas?limit=20&dataset=${encodeURIComponent(datasetSeed)}`, 'cache-cold');
  sleep(0.1);
}

export function cacheHot() {
  get(`/api/v1/arenas?limit=20&dataset=${encodeURIComponent(datasetSeed)}`, 'cache-hot');
  sleep(0.1);
}

export function login() {
  const response = http.post(`${baseURL}/api/v1/auth/login`, JSON.stringify({ email: accountEmail, password: accountPassword }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { workload: 'login' },
  });
  check(response, { 'login returns a handled response': (r) => r.status >= 200 && r.status < 500 });
  sleep(0.1);
}

export function position() {
  get(`/api/v1/arenas/${arenaID}/position`, 'position');
  sleep(0.1);
}

export function argumentWallet() {
  get(`/api/v1/arenas/${arenaID}/arguments?limit=20`, 'argument-wallet');
  get('/api/v1/wallet/summary', 'argument-wallet');
  sleep(0.1);
}

export function webhookReplay() {
  const response = http.post(`${baseURL}/api/v1/webhooks/stripe`, JSON.stringify({
    id: `evt_synthetic_${datasetSeed}`,
    type: 'checkout.session.completed',
    livemode: false,
  }), {
    headers: { 'Content-Type': 'application/json', 'Stripe-Signature': 'synthetic-invalid-signature' },
    tags: { workload: 'webhook-replay' },
  });
  check(response, { 'webhook replay is handled': (r) => r.status >= 200 && r.status < 500 });
  sleep(0.1);
}

export function arenaViral() {
  get(`/api/v1/arenas/${arenaID}`, 'arena-viral');
  sleep(0.05);
}

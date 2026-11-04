import json, sys, time, urllib.request, urllib.error

AK = 'http://localhost:19000/api/v3'
H = {'Authorization': 'Bearer bootstrap-token-for-testing-0123456789', 'Content-Type': 'application/json', 'Accept': 'application/json'}

def ak(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(AK + path, data=data, method=method, headers=H)
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        return e.code, e.read()[:500]

for _ in range(100):
    flows = {f['slug']: f['pk'] for f in ak('GET', '/flows/instances/?page_size=100')[1]['results']}
    if 'default-provider-authorization-implicit-consent' in flows and 'default-provider-invalidation-flow' in flows:
        break
    time.sleep(3)
key = next(k for k in ak('GET', '/crypto/certificatekeypairs/?page_size=50')[1]['results'] if 'Self-signed' in k['name'])
scopes = [m['pk'] for m in ak('GET', '/propertymappings/provider/scope/?page_size=50')[1]['results']
          if m['managed'] in ('goauthentik.io/providers/oauth2/scope-openid', 'goauthentik.io/providers/oauth2/scope-email', 'goauthentik.io/providers/oauth2/scope-profile')]
print('flows ok:', 'default-provider-authorization-implicit-consent' in flows, 'scopes:', len(scopes))

s, provider = ak('POST', '/providers/oauth2/', {
    'name': 'Diffluence OIDC',
    'authorization_flow': flows['default-provider-authorization-implicit-consent'],
    'invalidation_flow': flows['default-provider-invalidation-flow'],
    'client_type': 'confidential', 'client_id': 'diffluence-client', 'client_secret': 'diffluence-secret',
    'redirect_uris': [{'matching_mode': 'strict', 'url': 'http://localhost:8080/api/v1/auth/oidc/callback'}],
    'signing_key': key['pk'], 'property_mappings': scopes, 'sub_mode': 'hashed_user_id', 'issuer_mode': 'per_provider',
    'grant_types': ['authorization_code', 'refresh_token'],
})
print('provider', s)
if s != 201:
    print(provider); sys.exit(1)
s, app = ak('POST', '/core/applications/', {'name': 'Diffluence login', 'slug': 'diffluence', 'provider': provider['pk']})
print('application', s, '' if s == 201 else app)

# One user only Authentik knows, and one who arrives through SCIM first.
for username, name, email in [('alice', 'Alice Fresh', 'alice@example.com'), ('carol', 'Carol Scim', 'carol@example.com')]:
    s, u = ak('POST', '/core/users/', {'username': username, 'name': name, 'email': email, 'is_active': True, 'path': 'users'})
    print('user', username, s)
    s, r = ak('POST', f"/core/users/{u['pk']}/set_password/", {'password': 'authentik-user-password-1'})
    print('password', s)

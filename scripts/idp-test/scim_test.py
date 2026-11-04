import json, subprocess, sys, time, urllib.request, urllib.error, http.cookiejar, os

AK = 'http://localhost:19000/api/v3'
DF = 'http://localhost:19080/api/v1'
HERE = os.path.dirname(os.path.abspath(__file__))
PASSWORD = 'correct horse battery staple'
AKH = {'Authorization': 'Bearer bootstrap-token-for-testing-0123456789', 'Content-Type': 'application/json', 'Accept': 'application/json'}
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
results = []

def call(op, method, url, body=None, headers=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    try:
        with op(req, timeout=60) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw)
        except ValueError:
            return e.code, raw[:300]

def ak(method, path, body=None):
    return call(urllib.request.urlopen, method, AK + path, body, AKH)

def df(method, path, body=None):
    return call(opener.open, method, DF + path, body, {'Content-Type': 'application/json'})

def check(name, ok, detail=''):
    results.append(ok)
    print(('PASS ' if ok else 'FAIL ') + name + ('' if ok else f'   -> {detail}'))

# Diffluence: an administrator and a token.
p = subprocess.run(['docker', 'compose', 'exec', '-T', 'diffluence', 'diffluence', 'user', 'create-admin', '--email', 'admin@example.com', '--name', 'Admin'],
                   input=PASSWORD + '\n', text=True, capture_output=True, cwd=HERE)
print(p.stdout.strip().splitlines()[-1] if p.stdout else p.stderr)
s, r = df('POST', '/auth/login', {'email': 'admin@example.com', 'password': PASSWORD})
assert s == 200, r
s, r = df('POST', '/admin/scim-tokens', {'name': 'Authentik'})
assert s == 201, r
token = r['secret']

# Authentik: the provider, as an administrator would set it up.
# Authentik creates its default mappings in the background after it starts.
for _ in range(100):
    s, r = ak('GET', '/propertymappings/provider/scim/?page_size=50')
    maps = {m['managed']: m['pk'] for m in r['results']}
    if 'goauthentik.io/providers/scim/user' in maps and 'goauthentik.io/providers/scim/group' in maps:
        break
    time.sleep(3)
s, provider = ak('POST', '/providers/scim/', {
    'name': 'Diffluence', 'url': 'http://diffluence:8080/scim/v2', 'token': token,
    'property_mappings': [maps['goauthentik.io/providers/scim/user']],
    'property_mappings_group': [maps['goauthentik.io/providers/scim/group']],
    'exclude_users_service_account': True,
})
assert s == 201, provider
pid = provider['pk']
s, app = ak('POST', '/core/applications/', {'name': 'Diffluence', 'slug': 'diffluence-scim', 'backchannel_providers': [pid]})
assert s == 201, app

def sync(model, obj):
    s, r = ak('POST', f'/providers/scim/{pid}/sync/object/', {'sync_object_model': model, 'sync_object_id': str(obj)})
    return s, r

def diffluence_user(email):
    s, r = df('GET', '/admin/users?q=' + email)
    for u in r.get('users', []):
        if u['email'] == email:
            return u
    return None

def diffluence_group(name):
    s, r = df('GET', '/admin/groups')
    for g in r.get('groups', []):
        if g['name'] == name:
            return g
    return None

def wait_for(fn, seconds=120):
    end = time.time() + seconds
    while time.time() < end:
        v = fn()
        if v:
            return v
        time.sleep(1)
    return fn()

# 1. Create a user.
s, user = ak('POST', '/core/users/', {'username': 'jdoe', 'name': 'Jane Doe', 'email': 'jane@example.com', 'is_active': True, 'path': 'users'})
assert s == 201, user
print('sync user:', sync('authentik_core.user', user['pk'])[0])
u = wait_for(lambda: diffluence_user('jane@example.com'))
check('a user created in Authentik arrives', bool(u), u)
if u:
    check('  with name and source', u['display_name'] == 'Jane Doe' and u['source'] == 'scim', u)
    check('  active', u['status'] == 'active', u)

# 2. Update a user.
s, r = ak('PATCH', f"/core/users/{user['pk']}/", {'name': 'Jane Smith'})
print('sync user:', sync('authentik_core.user', user['pk'])[0])
u = wait_for(lambda: (lambda x: x if x and x['display_name'] == 'Jane Smith' else None)(diffluence_user('jane@example.com')))
check('a changed name arrives', bool(u), diffluence_user('jane@example.com'))

# 3. A group with a member.
s, group = ak('POST', '/core/groups/', {'name': 'Finance', 'users': [user['pk']]})
assert s == 201, group
print('sync group:', sync('authentik_core.group', group['pk'])[0])
g = wait_for(lambda: diffluence_group('Finance'))
check('a group created in Authentik arrives', bool(g), g)
if g:
    check('  read-only here', g['source'] == 'scim', g)
    m = wait_for(lambda: (lambda x: x if x['members'] == 1 else None)(diffluence_group('Finance')))
    check('  with its member', bool(m), diffluence_group('Finance'))

# 4. A second user joins, the first leaves.
s, user2 = ak('POST', '/core/users/', {'username': 'bob', 'name': 'Bob Stone', 'email': 'bob@example.com', 'is_active': True, 'path': 'users'})
print('sync user2:', sync('authentik_core.user', user2['pk'])[0])
wait_for(lambda: diffluence_user('bob@example.com'))
s, r = ak('POST', f"/core/groups/{group['pk']}/add_user/", {'pk': user2['pk']})
print('add_user:', s)
print('sync group:', sync('authentik_core.group', group['pk'])[0])
m = wait_for(lambda: (lambda x: x if x and x['members'] == 2 else None)(diffluence_group('Finance')))
check('a new member arrives', bool(m), diffluence_group('Finance'))
s, r = ak('POST', f"/core/groups/{group['pk']}/remove_user/", {'pk': user['pk']})
print('remove_user:', s)
print('sync group:', sync('authentik_core.group', group['pk'])[0])
m = wait_for(lambda: (lambda x: x if x and x['members'] == 1 else None)(diffluence_group('Finance')))
check('a member who leaves is removed', bool(m), diffluence_group('Finance'))

# 5. Deactivate.
s, r = ak('PATCH', f"/core/users/{user['pk']}/", {'is_active': False})
print('sync user:', sync('authentik_core.user', user['pk'])[0])
u = wait_for(lambda: (lambda x: x if x and x['status'] == 'disabled' else None)(diffluence_user('jane@example.com')))
check('deactivating in Authentik disables the account', bool(u), diffluence_user('jane@example.com'))

# 6. Delete a user and a group.
s, r = ak('DELETE', f"/core/users/{user2['pk']}/")
print('delete user:', s)
u = wait_for(lambda: (lambda x: x if x and x['status'] == 'disabled' else None)(diffluence_user('bob@example.com')))
check('deleting in Authentik disables the account here', bool(u), diffluence_user('bob@example.com'))
s, r = ak('DELETE', f"/core/groups/{group['pk']}/")
print('delete group:', s)
gone = wait_for(lambda: diffluence_group('Finance') is None)
check('deleting a group removes it', bool(gone), diffluence_group('Finance'))

# What Authentik recorded about the provider.
s, r = ak('GET', f'/providers/scim/{pid}/sync/status/')
print('sync status:', json.dumps(r)[:300])
s, r = df('GET', '/admin/audit-events?limit=500')
actions = {}
for e in r.get('events', []):
    if e.get('actor_id', '').startswith('scim:'):
        actions[e['action']] = actions.get(e['action'], 0) + 1
print('audit events by SCIM:', actions)
print(f'{sum(results)} of {len(results)} checks passed')
sys.exit(0 if all(results) else 1)

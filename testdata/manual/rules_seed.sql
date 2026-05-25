-- Seed data for manual testing of Cerberus with Istio + echoserver
-- Service: echoserver (fixed UUID for reproducibility)

-- Clean up existing seed data (safe to re-run)
DELETE FROM authorization_rule_tuple
WHERE rule_id IN (
    SELECT id FROM authorization_rule
    WHERE service_id = '00000000-0000-0000-0000-000000000001'
);
DELETE FROM authorization_rule
WHERE service_id = '00000000-0000-0000-0000-000000000001';

-- +-----------------------------------------------+
-- | Rule 1: GET /api/v1/groups/{groupId}           |
-- | Checks: user can "read" a specific group       |
-- +-----------------------------------------------+
INSERT INTO authorization_rule (id, service_id, method, segment_count, static_prefix, path_regex, priority)
VALUES (
           '00000000-0000-0000-0000-000000000010',
           '00000000-0000-0000-0000-000000000001',
           'GET',
           4,
           '/api/v1/groups/',
           '^/api/v1/groups/(?<groupId>[^/]+)$',
           10
       );

INSERT INTO authorization_rule_tuple (id, rule_id, user_resource_type, object_resource_type, object_resource_id, permission)
VALUES (
           '00000000-0000-0000-0000-000000000011',
           '00000000-0000-0000-0000-000000000010',
           'user',
           'group',
           '{groupId}',
           'read'
       ),
       (
           '00000000-0000-0000-0000-000000000012',
           '00000000-0000-0000-0000-000000000010',
           'user',
           'group',
           '{groupId}',
           'member'
       );

-- +-----------------------------------------------+
-- | Rule 2: POST /api/v1/groups/{groupId}/members  |
-- | Checks: user can "write" a specific group      |
-- +-----------------------------------------------+
INSERT INTO authorization_rule (id, service_id, method, segment_count, static_prefix, path_regex, priority)
VALUES (
           '00000000-0000-0000-0000-000000000020',
           '00000000-0000-0000-0000-000000000001',
           'POST',
           5,
           '/api/v1/groups/',
           '^/api/v1/groups/(?<groupId>[^/]+)/members$',
           10
       );

INSERT INTO authorization_rule_tuple (id, rule_id, user_resource_type, object_resource_type, object_resource_id, permission)
VALUES (
           '00000000-0000-0000-0000-000000000021',
           '00000000-0000-0000-0000-000000000020',
           'user',
           'group',
           '{groupId}',
           'write'
       );

-- +-----------------------------------------------+
-- | Rule 3: GET /api/v1/admin/**                   |
-- | Checks: user is "admin" on the platform object |
-- | (wildcard: matches any admin sub-path)         |
-- +-----------------------------------------------+
INSERT INTO authorization_rule (id, service_id, method, segment_count, static_prefix, path_regex, priority)
VALUES (
           '00000000-0000-0000-0000-000000000030',
           '00000000-0000-0000-0000-000000000001',
           'GET',
           3,
           '/api/v1/admin/',
           '^/api/v1/admin/.*$',
           20
       );

INSERT INTO authorization_rule_tuple (id, rule_id, user_resource_type, object_resource_type, object_resource_id, permission)
VALUES (
           '00000000-0000-0000-0000-000000000031',
           '00000000-0000-0000-0000-000000000030',
           'user',
           'platform',
           'default',
           'admin'
       );

-- +-----------------------------------------------------------+
-- | Rule 4: POST /api/v1/groups/{groupId}/members/{memberId}  |
-- | Checks: user can "write" a specific group                 |
-- +-----------------------------------------------------------+
INSERT INTO authorization_rule (id, service_id, method, segment_count, static_prefix, path_regex, priority)
VALUES (
           '00000000-0000-0000-0000-000000000040',
           '00000000-0000-0000-0000-000000000001',
           'POST',
           6,
           '/api/v1/groups/',
           '^/api/v1/groups/(?P<groupId>[^/]+)/members/(?<memberId>[^/]+)$',
           10
       );
INSERT INTO authorization_rule_tuple (id, rule_id, user_resource_type, object_resource_type, object_resource_id, permission)
VALUES (
           '00000000-0000-0000-0000-000000000041',
           '00000000-0000-0000-0000-000000000040',
           'user',
           'group',
           '{groupId}',
           'read'
       ),
       (
           '00000000-0000-0000-0000-000000000051',
           '00000000-0000-0000-0000-000000000040',
           'user',
           'membership',
           '{memberId}',
           'read'
       );

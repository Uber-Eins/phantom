import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Divider, Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';

import { activateOnKey } from '@/utils/a11y';

export function defaultXmcProfile(): Record<string, unknown> {
  return { username: '', uuid: '', texturesValue: '', texturesSignature: '' };
}

// xray-core #6487 replaced the XMC mask's `usernames` string list with
// Mojang-signed session profiles. A legacy username cannot be upgraded
// automatically because only Mojang can issue the texture signature.
export function migrateXmcSettings(settings: Record<string, unknown>): {
  next: Record<string, unknown>;
  changed: boolean;
} {
  const out: Record<string, unknown> = { ...settings };
  let changed = false;
  if (!Array.isArray(out.profiles) && Array.isArray(out.usernames)) {
    out.profiles = out.usernames
      .filter((name): name is string => typeof name === 'string' && name.trim() !== '')
      .map((name) => ({ ...defaultXmcProfile(), username: name }));
    changed = true;
  }
  if ('usernames' in out) {
    delete out.usernames;
    changed = true;
  }
  if (!Array.isArray(out.profiles)) {
    out.profiles = [];
    changed = true;
  }
  return { next: out, changed };
}

const XMC_UUID_PATTERN = /^(?:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|[0-9a-fA-F]{32})$/;
const XMC_USERNAME_PATTERN = /^[A-Za-z0-9_]{3,16}$/;

function validateXmcUsername(_rule: unknown, value: unknown): Promise<void> {
  if (typeof value === 'string' && XMC_USERNAME_PATTERN.test(value)) {
    return Promise.resolve();
  }
  return Promise.reject(new Error('3-16 characters, letters/digits/underscore only'));
}

function validateXmcUuid(_rule: unknown, value: unknown): Promise<void> {
  if (typeof value === 'string' && XMC_UUID_PATTERN.test(value.trim())) {
    return Promise.resolve();
  }
  return Promise.reject(new Error('Enter the profile UUID (dashed or 32 hex characters)'));
}

export function XmcProfilesList({ tcpFieldName }: { tcpFieldName: number }) {
  const { t } = useTranslation();
  return (
    <Form.List name={[tcpFieldName, 'settings', 'profiles']}>
      {(profiles, { add, remove }) => (
        <>
          <Form.Item
            label="Profiles"
            extra="Signed Minecraft session profiles; resolve the UUID by username, then fetch the profile with unsigned=false."
          >
            <Button
              type="primary"
              size="small"
              icon={<PlusOutlined />}
              aria-label={t('add')}
              onClick={() => add(defaultXmcProfile())}
            />
          </Form.Item>
          {profiles.map((profile, index) => (
            <div key={profile.key}>
              <Divider style={{ margin: 0 }}>
                Profile {index + 1}
                <DeleteOutlined
                  className="danger-icon"
                  role="button"
                  tabIndex={0}
                  aria-label={t('remove')}
                  onClick={() => remove(profile.name)}
                  onKeyDown={activateOnKey(() => remove(profile.name))}
                />
              </Divider>
              <Form.Item
                label="Username"
                name={[profile.name, 'username']}
                rules={[{ validator: validateXmcUsername }]}
              >
                <Input placeholder="Notch" />
              </Form.Item>
              <Form.Item
                label="UUID"
                name={[profile.name, 'uuid']}
                rules={[{ validator: validateXmcUuid }]}
              >
                <Input placeholder="069a79f4-44e9-4726-a5be-fca90e38aaf5" />
              </Form.Item>
              <Form.Item
                label="Textures Value"
                name={[profile.name, 'texturesValue']}
                rules={[{ required: true, message: 'Textures value is required' }]}
              >
                <Input.TextArea
                  autoSize={{ minRows: 2, maxRows: 4 }}
                  placeholder="Base64 value from the session profile"
                />
              </Form.Item>
              <Form.Item
                label="Textures Signature"
                name={[profile.name, 'texturesSignature']}
                rules={[{ required: true, message: 'Textures signature is required' }]}
              >
                <Input.TextArea
                  autoSize={{ minRows: 2, maxRows: 4 }}
                  placeholder="Base64 signature from the session profile"
                />
              </Form.Item>
            </div>
          ))}
        </>
      )}
    </Form.List>
  );
}

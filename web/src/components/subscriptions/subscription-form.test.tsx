import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SubscriptionForm } from './subscription-form';
import type { Subscription } from '@/api/subscriptions';
import { useI18nStore } from '@/i18n';

const SUBSCRIPTION: Subscription = {
  id: 'sub-1',
  name: 'Example subscription',
  enabled: true,
  status: 'ok',
  last_error: '',
  provider_name: 'provider-example',
  provider_source: 'linked',
  provider_exists: true,
  url_masked: 'https://sub.example.com/••••',
  node_count: 42,
  quota_bytes: 200 * 1024 ** 3,
  used_bytes: 50 * 1024 ** 3,
  remaining_bytes: 150 * 1024 ** 3,
  usage_source: 'vendor',
  local_used_bytes: null,
  expires_at: '2027-01-09T04:00:00Z',
  expires_source: 'vendor',
  reset_day: 10,
  reset_hour: 12,
  reset_minute: 0,
  timezone: 'Asia/Shanghai',
  cycle_start: '2026-10-10T04:00:00Z',
  cycle_end: '2026-11-10T04:00:00Z',
  next_reset: '2026-11-10T04:00:00Z',
  userinfo_fetched_at: '2026-10-21T03:00:00Z',
  last_refresh_at: '2026-10-21T03:00:00Z',
  last_success_at: '2026-10-21T03:00:00Z',
  stale: false,
  userinfo: {
    upload: 10 * 1024 ** 3,
    download: 40 * 1024 ** 3,
    total: 200 * 1024 ** 3,
    expire: 1800000000,
    fetched_at: '2026-10-21T03:00:00Z',
  },
  traffic_provider_id: '',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-21T03:00:00Z',
};

const PROVIDERS = [
  {
    name: 'provider-example',
    type: 'http',
    node_count: 42,
    linked_subscription_id: 'sub-1',
    linked: true,
  },
];

function renderForm(overrides: Partial<React.ComponentProps<typeof SubscriptionForm>> = {}) {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onOpenChange = vi.fn();
  const onRequestDelete = vi.fn();
  render(
    <SubscriptionForm
      open
      onOpenChange={onOpenChange}
      subscription={SUBSCRIPTION}
      providers={PROVIDERS}
      trafficProviders={[]}
      onSave={onSave}
      onRequestDelete={onRequestDelete}
      {...overrides}
    />
  );
  return { onSave, onOpenChange, onRequestDelete };
}

describe('SubscriptionForm', () => {
  beforeEach(() => {
    useI18nStore.setState({ locale: 'en' });
  });

  it('keeps the stored URL when editing without re-entering it', async () => {
    const user = userEvent.setup();
    const { onSave, onOpenChange } = renderForm();

    const url = screen.getByLabelText('Subscription URL');
    expect(url).toHaveValue('');
    expect(url).toHaveAttribute('placeholder', expect.stringContaining('Leave blank'));

    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input, id] = onSave.mock.calls[0];
    expect(id).toBe('sub-1');
    expect(input.url).toBeUndefined();
    expect(input.provider_name).toBe('provider-example');
    expect(input.manual_expire).toBeUndefined();
    expect(input.quota_bytes).toBeUndefined();
    expect(input.reset_day).toBe(10);
    expect(input.reset_hour).toBe(12);
    expect(input.timezone).toBe('Asia/Shanghai');
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('requires a URL when the provider does not exist yet', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({ subscription: null, providers: [] });

    await user.type(screen.getByLabelText('Name'), 'New subscription');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText(/URL is required/i)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('blocks linking a provider that belongs to another subscription', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      providers: [
        { ...PROVIDERS[0], linked_subscription_id: 'sub-other', linked: true },
      ],
    });

    expect(screen.getByText(/already linked to another subscription/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it('preserves an unknown reset policy when only the name changes', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      subscription: {
        ...SUBSCRIPTION,
        reset_day: 0,
        reset_hour: 0,
        reset_minute: 0,
        timezone: null,
      },
    });

    const toggle = screen.getByLabelText('Configure monthly reset schedule');
    expect(toggle).not.toBeChecked();
    expect(screen.getByLabelText('Reset day')).toBeDisabled();

    const name = screen.getByLabelText('Name');
    await user.clear(name);
    await user.type(name, 'Renamed only');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.reset_day).toBe(0);
    expect(input.reset_hour).toBe(0);
    expect(input.reset_minute).toBe(0);
    expect(input.timezone).toBe('');
  });

  it('sends an explicitly configured schedule', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      subscription: {
        ...SUBSCRIPTION,
        reset_day: 0,
        reset_hour: 0,
        reset_minute: 0,
        timezone: null,
      },
    });

    await user.click(screen.getByLabelText('Configure monthly reset schedule'));
    const day = screen.getByLabelText('Reset day');
    await user.clear(day);
    await user.type(day, '10');
    const hour = screen.getByLabelText('Hour');
    await user.clear(hour);
    await user.type(hour, '12');
    const timezone = screen.getByLabelText('Timezone');
    await user.clear(timezone);
    await user.type(timezone, 'Asia/Shanghai');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.reset_day).toBe(10);
    expect(input.reset_hour).toBe(12);
    expect(input.reset_minute).toBe(0);
    expect(input.timezone).toBe('Asia/Shanghai');
  });

  it('explicitly clears a known schedule when the choice is unchecked', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm();

    const toggle = screen.getByLabelText('Configure monthly reset schedule');
    expect(toggle).toBeChecked();
    await user.click(toggle);
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.reset_day).toBe(0);
    expect(input.reset_hour).toBe(0);
    expect(input.reset_minute).toBe(0);
    expect(input.timezone).toBe('');
  });

  it('prefills a manual expiry but not a vendor expiry', () => {
    renderForm({
      subscription: {
        ...SUBSCRIPTION,
        expires_source: 'manual',
        expires_at: '2027-01-09T00:00:00Z',
      },
    });
    expect(screen.getByLabelText('Manual expiry (date, time & timezone)')).toHaveValue('2027-01-09T08:00');
  });

  it('keeps a manual-only quota when editing another field', () => {
    renderForm({
      subscription: {
        ...SUBSCRIPTION,
        userinfo: { upload: 0, download: 0, total: 0, expire: 0, fetched_at: '' },
        quota_bytes: 500 * 1024 ** 3,
      },
    });
    expect(screen.getByLabelText('Manual monthly quota (GiB)')).toHaveValue(500);
  });

  it('preserves untouched vendor/manual supplements when renaming', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      subscription: {
        ...SUBSCRIPTION,
        quota_bytes: 300 * 1024 ** 3,
        userinfo: {
          upload: 10 * 1024 ** 3,
          download: 40 * 1024 ** 3,
          total: 200 * 1024 ** 3,
          expire: 0,
          fetched_at: '2026-10-21T03:00:00Z',
        },
        expires_source: 'manual',
        expires_at: '2027-01-09T04:30:15Z',
      },
    });

    expect(screen.getByLabelText('Manual expiry (date, time & timezone)')).toHaveValue(
      '2027-01-09T12:30:15'
    );
    const name = screen.getByLabelText('Name');
    await user.clear(name);
    await user.type(name, 'Renamed');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.quota_bytes).toBeUndefined();
    expect(input.manual_expire).toBeUndefined();
  });

  it('sends an explicitly edited expiry as datetime in the chosen timezone', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      subscription: {
        ...SUBSCRIPTION,
        expires_source: 'manual',
        expires_at: '2027-01-09T04:30:15Z',
      },
    });

    fireEvent.change(screen.getByLabelText('Manual expiry (date, time & timezone)'), {
      target: { value: '2027-02-03T04:05:00' },
    });
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.manual_expire).toBe('2027-02-02T20:05:00.000Z');
  });

  it('explicitly clears quota and expiry supplements', async () => {
    const user = userEvent.setup();
    const { onSave } = renderForm({
      subscription: {
        ...SUBSCRIPTION,
        userinfo: { upload: 0, download: 0, total: 0, expire: 0, fetched_at: '' },
        quota_bytes: 500 * 1024 ** 3,
        expires_source: 'manual',
        expires_at: '2027-01-09T04:30:15Z',
      },
    });

    fireEvent.change(screen.getByLabelText('Manual monthly quota (GiB)'), {
      target: { value: '' },
    });
    fireEvent.change(screen.getByLabelText('Manual expiry (date, time & timezone)'), {
      target: { value: '' },
    });
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [input] = onSave.mock.calls[0];
    expect(input.quota_bytes).toBe(0);
    expect(input.manual_expire).toBe('');
  });

  it('does not prefill the vendor expiry into the manual field', () => {
    renderForm();
    expect(screen.getByLabelText('Manual expiry (date, time & timezone)')).toHaveValue('');
    expect(screen.getByText(/Vendor expiry/)).toBeInTheDocument();
  });
});

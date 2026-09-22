import { useState } from 'react';
import { Modal, TextInput, Select, SelectItem, InlineNotification } from '@carbon/react';

interface AddInstanceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAdded: () => void;
}

export function AddInstanceModal({ isOpen, onClose, onAdded }: AddInstanceModalProps) {
  const [formData, setFormData] = useState({
    id: '',
    type: 'postgres',
    dsn: '',
    interval: '15s'
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    setLoading(true);
    setError('');

    try {
      const apiUrl = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${apiUrl}/api/instances`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(formData)
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to add instance');
      }

      onAdded();
      onClose();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      open={isOpen}
      modalHeading="Add Database Instance"
      primaryButtonText={loading ? 'Adding...' : 'Add Instance'}
      secondaryButtonText="Cancel"
      onRequestClose={onClose}
      onRequestSubmit={handleSubmit}
      primaryButtonDisabled={loading || !formData.id || !formData.dsn}
    >
      {error && (
        <InlineNotification
          kind="error"
          title="Error"
          subtitle={error}
          onCloseButtonClick={() => setError('')}
          style={{ marginBottom: '1rem' }}
        />
      )}
      
      <div style={{ marginBottom: '1rem' }}>
        <TextInput
          id="instance-id"
          labelText="Instance ID"
          placeholder="e.g., prod-db-1"
          value={formData.id}
          onChange={(e) => setFormData({ ...formData, id: e.target.value })}
        />
      </div>

      <div style={{ marginBottom: '1rem' }}>
        <Select
          id="db-type"
          labelText="Database Type"
          value={formData.type}
          onChange={(e) => setFormData({ ...formData, type: e.target.value })}
        >
          <SelectItem value="postgres" text="PostgreSQL" />
          <SelectItem value="mysql" text="MySQL" />
          <SelectItem value="mongodb" text="MongoDB" />
        </Select>
      </div>

      <div style={{ marginBottom: '1rem' }}>
        <TextInput
          id="dsn"
          labelText="Connection String (DSN)"
          placeholder={formData.type === 'postgres' ? 'postgres://user:pass@host:5432/db' : formData.type === 'mysql' ? 'user:pass@tcp(host:3306)/db' : 'mongodb://user:pass@host:27017/'}
          value={formData.dsn}
          onChange={(e) => setFormData({ ...formData, dsn: e.target.value })}
        />
      </div>

      <div style={{ marginBottom: '1rem' }}>
        <Select
          id="interval"
          labelText="Collection Interval"
          value={formData.interval}
          onChange={(e) => setFormData({ ...formData, interval: e.target.value })}
        >
          <SelectItem value="5s" text="5 seconds" />
          <SelectItem value="15s" text="15 seconds" />
          <SelectItem value="30s" text="30 seconds" />
          <SelectItem value="1m" text="1 minute" />
        </Select>
      </div>
    </Modal>
  );
}

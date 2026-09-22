import { useEffect, useState, useRef } from 'react';
import { Add, TrashCan, Db2Database } from '@carbon/icons-react';
import { 
  Header, HeaderName, HeaderGlobalBar, HeaderGlobalAction,
  Grid, Column, Tile, ClickableTile, Tag, ProgressBar, Button,
  DataTable, Table, TableHead, TableRow, TableHeader, TableBody, TableCell,
  Dropdown
} from '@carbon/react';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { Routes, Route, useNavigate, useMatch } from 'react-router-dom';
import { AddInstanceModal } from './components/AddInstanceModal';

interface Metric {
  instance_id: string;
  db_type: string;
  timestamp: string;
  name: string;
  value: number;
  labels?: Record<string, string>;
}

function extractDbName(dsn: string): string {
  if (!dsn) return 'Unknown DB';
  try {
    const withoutQuery = dsn.split('?')[0];
    const parts = withoutQuery.split('/');
    const dbPart = parts[parts.length - 1];
    
    if (!dbPart) {
      if (dsn.startsWith('mongo')) return 'MongoDB Cluster';
      return 'Unknown DB';
    }
    return dbPart;
  } catch (e) {
    // fallback
  }
  return 'Unknown DB';
}

function MetricCard({ label, value, unit, helperText, status }: any) {
  const tagType = { healthy: 'green', warning: 'yellow', critical: 'red' }[status as 'healthy' | 'warning' | 'critical'] || 'blue';
  const tagLabel = { healthy: 'Saudável', warning: 'Atenção', critical: 'Crítico' }[status as 'healthy' | 'warning' | 'critical'] || 'Info';

  return (
    <Tile>
      <p style={{ fontSize: '0.875rem', color: 'var(--cds-text-secondary)', marginBottom: '0.5rem' }}>
        {label}
      </p>
      <h2 style={{ fontFamily: "'Roboto Mono', monospace", fontSize: '2rem', margin: 0 }}>
        {value}{unit && <span style={{ fontSize: '1rem' }}> {unit}</span>}
      </h2>
      {helperText && (
        <p style={{ fontSize: '0.75rem', color: 'var(--cds-text-placeholder)', marginTop: '0.25rem' }}>
          {helperText}
        </p>
      )}
      <Tag type={tagType as any} size="sm" style={{ marginTop: '0.75rem' }}>{tagLabel}</Tag>
    </Tile>
  );
}

function App() {
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [activeConnections, setActiveConnections] = useState<any[]>([]);
  const [topQueries, setTopQueries] = useState<Metric[]>([]);
  const [instances, setInstances] = useState<any[]>([]);
  const navigate = useNavigate();
  const match = useMatch("/db/:instanceId");
  const selectedInstance = match?.params.instanceId || '';

  const setSelectedInstance = (id: string) => {
    if (!id) {
      navigate('/');
    } else {
      navigate(`/db/${id}`);
    }
  };

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeQueries, setActiveQueries] = useState<any[]>([]);
  const [tpsHistory, setTpsHistory] = useState<any[]>([]);
  const lastTotalXactRef = useRef<{ [key: string]: { value: number; time: number } }>({});

  const fetchInstances = async () => {
    try {
      const apiUrl = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${apiUrl}/api/instances`);
      if (!res.ok) {
        throw new Error(`Server returned ${res.status}`);
      }
      const rawData = await res.json();
      const data = rawData.map((d: any) => ({
        id: d.id || d.ID,
        type: d.type || d.Type,
        dsn: d.dsn || d.DSN,
        interval: d.interval || d.Interval
      }));
      setInstances(data);
    } catch (err) {
      console.error('Failed to fetch instances', err);
    }
  };

  const handleDeleteInstance = async () => {
    if (!selectedInstance) return;
    
    if (window.confirm(`Are you sure you want to delete the database instance "${selectedInstance}"?`)) {
      try {
        const apiUrl = import.meta.env.VITE_API_URL || '';
        const res = await fetch(`${apiUrl}/api/instances/${selectedInstance}`, {
          method: 'DELETE'
        });
        
        if (!res.ok) {
          throw new Error('Failed to delete instance');
        }
        
        navigate('/');
        fetchInstances();
      } catch (err) {
        console.error('Error deleting instance', err);
        alert('Failed to delete instance');
      }
    }
  };

  useEffect(() => {
    fetchInstances();
  }, []);
  
  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = import.meta.env.VITE_API_URL 
      ? import.meta.env.VITE_API_URL.replace('http', 'ws') + '/ws/metrics'
      : `${protocol}//${window.location.host}/ws/metrics`;

    const ws = new WebSocket(wsUrl);
    
    ws.onmessage = (event) => {
      const metric: Metric = JSON.parse(event.data);
      
      setMetrics(prev => [...prev.slice(-100), metric]);
      
      if (metric.name === 'active_connections') {
        setActiveConnections(prev => {
          const newPoint = { 
            instance_id: metric.instance_id,
            time: new Date(metric.timestamp).toLocaleTimeString(), 
            value: metric.value 
          };
          return [...prev.slice(-100), newPoint];
        });
      }

      if (metric.name === 'top_query_time') {
        setTopQueries(prev => {
          const filtered = prev.filter(q => q.labels?.query !== metric.labels?.query);
          return [...filtered, metric].sort((a, b) => b.value - a.value).slice(0, 5);
        });
      }

      if (metric.name === 'active_query') {
        setActiveQueries(prev => {
          const filtered = prev.filter(q => q.labels?.query !== metric.labels?.query);
          return [metric, ...filtered].slice(0, 15);
        });
      }

      if (metric.name === 'total_transactions') {
        const nowMs = new Date(metric.timestamp).getTime();
        const prev = lastTotalXactRef.current[metric.instance_id];
        
        if (prev) {
          const deltaSecs = (nowMs - prev.time) / 1000;
          if (deltaSecs > 0) {
            const tps = Math.max(0, (metric.value - prev.value) / deltaSecs);
            setTpsHistory(prevHistory => {
              const newPoint = {
                instance_id: metric.instance_id,
                time: new Date(metric.timestamp).toLocaleTimeString(),
                value: Math.round(tps)
              };
              return [...prevHistory.slice(-100), newPoint];
            });
          }
        }
        
        lastTotalXactRef.current[metric.instance_id] = { value: metric.value, time: nowMs };
      }
    };

    return () => ws.close();
  }, []);

  const filteredConnections = activeConnections.filter(c => c.instance_id === selectedInstance);
  const currentConnections = filteredConnections.length > 0 
    ? filteredConnections[filteredConnections.length - 1].value 
    : 0;

  const currentCacheHit = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'cache_hit_ratio')
    .slice(-1)[0]?.value?.toFixed(2) || '0.00';

  const currentConnectionsPct = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'connections_pct')
    .slice(-1)[0]?.value?.toFixed(1) || null;

  const currentDiskUsage = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'disk_usage_pct')
    .slice(-1)[0]?.value?.toFixed(1) || null;

  const currentLocks = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'locks_waiting')
    .slice(-1)[0]?.value || 0;

  const currentDeadlocks = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'deadlocks_count')
    .slice(-1)[0]?.value || 0;

  const currentLag = metrics
    .filter(m => m.instance_id === selectedInstance && m.name === 'replication_lag_seconds')
    .slice(-1)[0]?.value;

  const filteredTopQueries = topQueries.filter(q => q.instance_id === selectedInstance);
  const filteredTps = tpsHistory.filter(t => t.instance_id === selectedInstance);

  const tableHeaders = [
    { key: 'status', header: 'Status' },
    { key: 'query', header: 'Consulta' },
    { key: 'user', header: 'Usuário' },
    { key: 'totalTime', header: 'Tempo total (ms)' },
  ];

  const tableRows = filteredTopQueries.map((q, idx) => ({
    id: idx.toString(),
    status: q.value > 1000 ? 'red' : q.value > 500 ? 'yellow' : 'green',
    query: q.labels?.query || 'Unknown',
    user: q.labels?.username ? `${q.labels.username} ${q.labels.is_admin === 'true' ? '(Admin)' : ''}` : '-',
    totalTime: q.value.toFixed(2),
  }));

  const filteredActiveQueries = activeQueries.filter(q => q.instance_id === selectedInstance);
  const activeTableHeaders = [
    { key: 'state', header: 'Status' },
    { key: 'query', header: 'Consulta' },
    { key: 'user', header: 'Usuário' },
    { key: 'duration', header: 'Duração (s)' },
    { key: 'time', header: 'Visto às' },
  ];
  const activeTableRows = filteredActiveQueries.map((q, idx) => ({
    id: idx.toString(),
    state: q.labels?.state || 'active',
    query: q.labels?.query || 'Unknown',
    user: q.labels?.username || '-',
    duration: q.value.toFixed(2),
    time: new Date(q.timestamp).toLocaleTimeString(),
  }));

  return (
    <div className="cds--white">
      <Header aria-label="DBWatch">
        <HeaderName href="#" prefix="" onClick={(e: any) => { e.preventDefault(); setSelectedInstance(''); }} style={{ fontSize: '1.5rem', fontWeight: 800 }}>
          DBWatch
        </HeaderName>
        <HeaderGlobalBar>
          <HeaderGlobalAction aria-label="Add Database" onClick={() => setIsModalOpen(true)}>
            <Add size={32} />
          </HeaderGlobalAction>
        </HeaderGlobalBar>
      </Header>

      <div className="dbwatch-dashboard">
        <AddInstanceModal 
          isOpen={isModalOpen} 
          onClose={() => setIsModalOpen(false)} 
          onAdded={() => {
            fetchInstances();
          }}
        />

        <Routes>
          <Route path="/" element={
          <div style={{ paddingBottom: '2rem' }}>
            <Grid>
              <Column lg={16} md={8} sm={4}>
                <h2 style={{ marginBottom: '1.5rem', fontWeight: 600 }}>Bancos de Dados</h2>
              </Column>
            </Grid>
            <Grid>
              {instances.map(inst => (
                <Column lg={4} md={4} sm={4} key={inst.id} style={{ marginBottom: '1rem' }}>
                  <ClickableTile 
                    onClick={() => setSelectedInstance(inst.id)} 
                    style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
                  >
                    <Db2Database size={32} style={{ color: 'var(--cds-interactive-01)', marginBottom: '1rem' }} />
                    <h4 style={{ fontWeight: 600, marginBottom: '0.5rem' }}>{inst.id}</h4>
                    <p style={{ color: 'var(--cds-text-secondary)', fontSize: '0.875rem', marginBottom: '1rem', wordBreak: 'break-all' }}>
                      {extractDbName(inst.dsn)}
                    </p>
                    <div style={{ marginTop: 'auto' }}>
                      <Tag type="blue" size="sm">{inst.type}</Tag>
                    </div>
                  </ClickableTile>
                </Column>
              ))}
              <Column lg={4} md={4} sm={4} style={{ marginBottom: '1rem' }}>
                <ClickableTile 
                  onClick={() => setIsModalOpen(true)}
                  style={{ 
                    height: '100%', 
                    minHeight: '180px', 
                    display: 'flex', 
                    flexDirection: 'column', 
                    alignItems: 'center', 
                    justifyContent: 'center',
                    border: '2px dashed var(--cds-border-strong)',
                    backgroundColor: 'transparent'
                  }}
                >
                  <Add style={{ width: '48px', height: '48px', marginBottom: '1.5rem', color: 'var(--cds-link-primary)' }} />
                  <span style={{ color: 'var(--cds-link-primary)', fontWeight: 700, fontSize: '1.25rem' }}>Add Database</span>
                </ClickableTile>
              </Column>
            </Grid>
          </div>
          } />
          
          <Route path="/db/:instanceId" element={
          <>
            <Grid style={{ marginBottom: '2rem' }}>
              <Column lg={16} md={8} sm={4}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div style={{ width: '300px' }}>
                    <Dropdown
                      id="instance-select"
                      titleText="Select Database Instance"
                      label="Choose an instance"
                      items={instances}
                      itemToString={(item: any) => (item ? `${item.id} - ${extractDbName(item.dsn)}` : '')}
                      selectedItem={instances.find(i => i.id === selectedInstance) || null}
                      onChange={({ selectedItem }) => setSelectedInstance(selectedItem?.id || '')}
                    />
                  </div>
                  <Button 
                    kind="danger--tertiary" 
                    renderIcon={TrashCan} 
                    onClick={handleDeleteInstance}
                    size="md"
                  >
                    Delete Instance
                  </Button>
                </div>
              </Column>
            </Grid>

            <Grid style={{ marginBottom: '2rem' }}>
              <Column lg={3} md={2} sm={4}>
                <MetricCard 
                  label="Active Connections" 
                  value={currentConnections} 
                  unit={currentConnectionsPct ? `(${currentConnectionsPct}%)` : ''}
                  helperText="Current pool utilization" 
                  status={Number(currentConnectionsPct) > 80 ? 'warning' : 'healthy'} 
                />
                {currentConnectionsPct && (
                  <ProgressBar 
                    label="" 
                    value={Math.min(100, Number(currentConnectionsPct))} 
                    max={100} 
                    className="custom-progress" 
                  />
                )}
              </Column>
              <Column lg={3} md={2} sm={4}>
                <MetricCard 
                  label="Cache Hit Ratio" 
                  value={currentCacheHit} 
                  unit="%" 
                  helperText="Healthy read performance" 
                  status={Number(currentCacheHit) < 95 ? 'warning' : 'healthy'} 
                />
              </Column>
              <Column lg={3} md={2} sm={4}>
                <MetricCard 
                  label="Disk Usage" 
                  value={currentDiskUsage !== null ? currentDiskUsage : '0.0'} 
                  unit="%" 
                  helperText="Based on 500MB limit" 
                  status={Number(currentDiskUsage) > 90 ? 'critical' : Number(currentDiskUsage) > 80 ? 'warning' : 'healthy'} 
                />
                {currentDiskUsage !== null && (
                  <ProgressBar 
                    label="" 
                    value={Math.min(100, Number(currentDiskUsage))} 
                    max={100} 
                    className="custom-progress" 
                  />
                )}
              </Column>
              <Column lg={3} md={2} sm={4}>
                <MetricCard 
                  label="Slowest Query" 
                  value={filteredTopQueries[0] ? (filteredTopQueries[0].value / 1000).toFixed(2) : '0'} 
                  unit="s" 
                  helperText="Needs optimization" 
                  status={filteredTopQueries[0] && filteredTopQueries[0].value > 1000 ? 'warning' : 'healthy'} 
                />
              </Column>
            </Grid>

            <Grid style={{ marginBottom: '2rem' }}>
              <Column lg={12} md={8} sm={4}>
                <Tile>
                  <h3 style={{ marginBottom: '1rem', fontWeight: 600 }}>Active Connections Trend</h3>
                  <div className="chart-container">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={filteredConnections}>
                        <defs>
                          <linearGradient id="colorValue" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--cds-support-info)" stopOpacity={0.3}/>
                            <stop offset="95%" stopColor="var(--cds-support-info)" stopOpacity={0}/>
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke="var(--cds-border-subtle)" />
                        <XAxis dataKey="time" stroke="var(--cds-text-secondary)" fontSize={12} />
                        <YAxis stroke="var(--cds-text-secondary)" fontSize={12} />
                        <Tooltip contentStyle={{ backgroundColor: 'var(--cds-layer-01)', borderColor: 'var(--cds-border-subtle)', color: 'var(--cds-text-primary)' }} />
                        <Area type="monotone" dataKey="value" stroke="var(--cds-support-info)" fillOpacity={1} fill="url(#colorValue)" />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                </Tile>
                <Tile style={{ marginTop: '2rem' }}>
                  <h3 style={{ marginBottom: '1rem', fontWeight: 600 }}>Transactions per Second (TPS)</h3>
                  <div className="chart-container">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={filteredTps}>
                        <defs>
                          <linearGradient id="colorTps" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--cds-support-success)" stopOpacity={0.3}/>
                            <stop offset="95%" stopColor="var(--cds-support-success)" stopOpacity={0}/>
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke="var(--cds-border-subtle)" />
                        <XAxis dataKey="time" stroke="var(--cds-text-secondary)" fontSize={12} />
                        <YAxis stroke="var(--cds-text-secondary)" fontSize={12} />
                        <Tooltip contentStyle={{ backgroundColor: 'var(--cds-layer-01)', borderColor: 'var(--cds-border-subtle)', color: 'var(--cds-text-primary)' }} />
                        <Area type="monotone" dataKey="value" stroke="var(--cds-support-success)" fillOpacity={1} fill="url(#colorTps)" />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                </Tile>
              </Column>
              <Column lg={4} md={4} sm={4}>
                <Tile style={{ height: '100%', border: currentLocks > 0 ? '2px solid var(--cds-support-error)' : 'none' }}>
                  <h3 style={{ marginBottom: '1rem', fontWeight: 600 }}>Lock Contention</h3>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1rem' }}>
                    <h2 style={{ fontSize: '3rem', margin: 0, color: currentLocks > 0 ? 'var(--cds-support-error)' : 'var(--cds-text-primary)' }}>
                      {currentLocks}
                    </h2>
                    <span style={{ color: 'var(--cds-text-secondary)' }}>waiting</span>
                  </div>
                  <Tag type={currentLocks > 0 ? 'red' : 'green'}>
                    {currentDeadlocks} deadlocks detected
                  </Tag>
                  {currentLag !== undefined && (
                    <div style={{ marginTop: '2rem' }}>
                      <h3 style={{ marginBottom: '0.5rem', fontWeight: 600 }}>Replication Lag</h3>
                      <h2 style={{ fontSize: '2rem', margin: 0 }}>{currentLag}s</h2>
                    </div>
                  )}
                </Tile>
              </Column>
            </Grid>

            <Grid style={{ marginBottom: '2rem' }}>
              <Column lg={16} md={8} sm={4}>
                <Tile>
                  <h3 style={{ marginBottom: '1.5rem', fontWeight: 600 }}>Top Queries & Latest Activity</h3>
                  <DataTable rows={tableRows} headers={tableHeaders}>
                    {({ rows, headers, getHeaderProps, getRowProps, getTableProps }) => (
                    <Table {...getTableProps()}>
                      <TableHead>
                        <TableRow>
                          {headers.map((header) => (
                            <TableHeader {...getHeaderProps({ header })} key={header.key}>
                              {header.header}
                            </TableHeader>
                          ))}
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {rows.map((row) => (
                          <TableRow {...getRowProps({ row })} key={row.id}>
                            {row.cells.map((cell) => (
                              <TableCell key={cell.id}>
                                {cell.info.header === 'status' ? (
                                  <Tag type={cell.value as any} size="sm">{cell.value === 'red' ? 'Crítico' : cell.value === 'yellow' ? 'Atenção' : 'Normal'}</Tag>
                                ) : cell.info.header === 'query' ? (
                                  <span className="query-cell">{cell.value}</span>
                                ) : (
                                  <span className="metric-value">{cell.value}</span>
                                )}
                              </TableCell>
                            ))}
                          </TableRow>
                        ))}
                        {rows.length === 0 && (
                          <TableRow>
                            <TableCell colSpan={3} style={{ textAlign: 'center' }}>Nenhuma query lenta detectada.</TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  )}
                </DataTable>
                </Tile>
              </Column>
            </Grid>

            <Grid style={{ marginBottom: '2rem' }}>
              <Column lg={16} md={8} sm={4}>
                <Tile>
                  <h3 style={{ marginBottom: '1.5rem', fontWeight: 600 }}>Sessões Ativas (Latest Queries)</h3>
                  <DataTable rows={activeTableRows} headers={activeTableHeaders}>
                    {({ rows, headers, getHeaderProps, getRowProps, getTableProps }) => (
                    <Table {...getTableProps()}>
                      <TableHead>
                        <TableRow>
                          {headers.map((header) => (
                            <TableHeader {...getHeaderProps({ header })} key={header.key}>
                              {header.header}
                            </TableHeader>
                          ))}
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {rows.map((row) => (
                          <TableRow {...getRowProps({ row })} key={row.id}>
                            {row.cells.map((cell) => (
                              <TableCell key={cell.id}>
                                {cell.info.header === 'state' ? (
                                  <Tag type="blue" size="sm">{cell.value}</Tag>
                                ) : cell.info.header === 'query' ? (
                                  <span className="query-cell" style={{ wordBreak: 'break-all' }}>{cell.value}</span>
                                ) : (
                                  <span className="metric-value">{cell.value}</span>
                                )}
                              </TableCell>
                            ))}
                          </TableRow>
                        ))}
                        {rows.length === 0 && (
                          <TableRow>
                            <TableCell colSpan={5} style={{ textAlign: 'center' }}>Nenhuma query ativa detectada no momento.</TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  )}
                </DataTable>
                </Tile>
              </Column>
            </Grid>
          </>
          } />
        </Routes>
      </div>
    </div>
  );
}

export default App;

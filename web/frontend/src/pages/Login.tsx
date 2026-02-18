import { useState, useEffect, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { LogIn } from 'lucide-react';
import { Button } from '../components/Button';
import { Input } from '../components/Input';
import { useAuth } from '../contexts/AuthContext';
import { useDesign } from '../contexts/DesignContext';
import { api } from '../lib/api';

export function Login() {
  const navigate = useNavigate();
  const { login } = useAuth();
  const { bgImage } = useDesign();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [rememberMe, setRememberMe] = useState(true);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [checkingSetup, setCheckingSetup] = useState(true);

  useEffect(() => {
    api.get<{ needs_setup: boolean }>('/setup/status')
      .then(data => {
        if (data.needs_setup) {
          setNeedsSetup(true);
        }
      })
      .catch(() => {})
      .finally(() => setCheckingSetup(false));
  }, []);

  useEffect(() => {
    if (needsSetup && !checkingSetup) {
      navigate('/setup', { replace: true });
    }
  }, [needsSetup, checkingSetup, navigate]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    if (!username.trim() || !password) {
      setError('Please enter your credentials');
      return;
    }
    setLoading(true);
    try {
      await login(username, password, rememberMe);
      navigate('/chat');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Invalid credentials');
    } finally {
      setLoading(false);
    }
  };

  if (checkingSetup) return null;

  return (
    <div className="min-h-screen flex items-center justify-center p-4 relative">
      <div
        className="absolute inset-0 bg-cover bg-center bg-no-repeat"
        style={{ backgroundImage: `url(${bgImage || '/preset-bg/bg-1.webp'})` }}
      />
      <div className="absolute inset-0 bg-black/85" />

      <div className="w-full max-w-sm relative z-10">
        <div className="text-center mb-8">
          <img
            src="/icon.webp"
            alt="OpenPaw"
            className="w-20 h-20 mx-auto mb-4 drop-shadow-[0_0_20px_rgba(232,75,165,0.3)]"
          />
          <h1 className="text-2xl font-bold text-text-0">OpenPaw</h1>
          <p className="text-sm text-text-3 mt-1">Sign in to your dashboard</p>
        </div>

        <div className="rounded-2xl border border-border-0 bg-surface-1/90 backdrop-blur-sm shadow-xl p-6">
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div role="alert" aria-live="assertive" className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-sm text-red-400">
                {error}
              </div>
            )}
            <Input
              label="Username"
              value={username}
              onChange={e => setUsername(e.target.value)}
              placeholder="Enter username"
              autoFocus
              autoComplete="username"
            />
            <Input
              label="Password"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="Enter password"
              autoComplete="current-password"
            />
            <label className="flex items-center gap-2 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={rememberMe}
                onChange={e => setRememberMe(e.target.checked)}
                className="w-4 h-4 rounded border-border-1 bg-surface-2 text-accent-primary focus:ring-accent-primary/50 focus:ring-offset-0 cursor-pointer"
              />
              <span className="text-sm text-text-2">Remember me for 30 days</span>
            </label>
            <Button type="submit" loading={loading} icon={<LogIn className="w-4 h-4" />} className="w-full">
              Sign In
            </Button>
          </form>
        </div>
      </div>
    </div>
  );
}

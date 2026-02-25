import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { ArrowRight, ArrowLeft, Check, Server, UserPlus, Sparkles, AlertTriangle, CheckCircle, Key, PawPrint } from 'lucide-react';
import { Button } from '../components/Button';
import { Input } from '../components/Input';
import { api, type User } from '../lib/api';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../components/Toast';

const PRESET_AVATARS = Array.from({ length: 45 }, (_, i) => `/avatars/avatar-${i + 1}.webp`);

export function Setup() {
  const navigate = useNavigate();
  const { login } = useAuth();
  const { toast } = useToast();
  const [step, setStep] = useState(0);
  const [loading, setLoading] = useState(false);

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [bindAddress, setBindAddress] = useState('0.0.0.0');
  const [port, setPort] = useState('41295');
  const [apiKey, setApiKey] = useState('');
  const enabledRoles = ['builder'];

  const [agentName, setAgentName] = useState('Pounce');
  const [agentAvatar, setAgentAvatar] = useState('/avatars/avatar-4.webp');
  const [agentGoal, setAgentGoal] = useState('');
  const [setupComplete, setSetupComplete] = useState(false);

  const [apiKeyConfigured, setApiKeyConfigured] = useState<boolean | null>(null);

  useEffect(() => {
    api.get<{ api_key_configured: boolean }>('/system/prerequisites')
      .then(data => {
        setApiKeyConfigured(data.api_key_configured);
      })
      .catch(() => setApiKeyConfigured(false));
  }, []);

  const [errors, setErrors] = useState<Record<string, string>>({});

  const validateStep1 = () => {
    const errs: Record<string, string> = {};
    if (!username.trim()) errs.username = 'Username is required';
    if (username.length < 3) errs.username = 'Username must be at least 3 characters';
    if (!password) errs.password = 'Password is required';
    if (password.length < 8) errs.password = 'Password must be at least 8 characters';
    if (password !== confirmPassword) errs.confirmPassword = 'Passwords do not match';
    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const validateStep3 = () => {
    const errs: Record<string, string> = {};
    const p = parseInt(port, 10);
    if (isNaN(p) || p < 1 || p > 65535) errs.port = 'Port must be 1-65535';
    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const handleNext = () => {
    if (step === 0) {
      setStep(1);
    } else if (step === 1) {
      if (validateStep1()) setStep(2);
    } else if (step === 2) {
      setStep(3);
    } else if (step === 3) {
      if (validateStep3()) handleInit();
    } else if (step === 4) {
      handlePersonalize();
    }
  };

  const handleInit = async () => {
    setLoading(true);
    try {
      await api.post<{ user: User }>('/setup/init', {
        username,
        password,
        bind_address: bindAddress,
        port: parseInt(port, 10),
        enabled_roles: enabledRoles,
        api_key: apiKey || undefined,
      });
      await login(username, password);
      setSetupComplete(true);
      setStep(4);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setLoading(false);
    }
  };

  const handlePersonalize = async () => {
    setLoading(true);
    try {
      await api.post('/setup/personalize', {
        name: agentName.trim() || 'Pounce',
        avatar_path: agentAvatar,
        goal: agentGoal.trim() || undefined,
      });
      navigate('/chat');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Personalization failed');
    } finally {
      setLoading(false);
    }
  };

  const steps = [
    {
      icon: <Sparkles className="w-6 h-6" />,
      title: 'Welcome to OpenPaw',
      content: (
        <div className="text-center space-y-4">
          <img
            src="/icon.webp"
            alt="OpenPaw"
            className="w-24 h-24 mx-auto drop-shadow-[0_0_24px_rgba(232,75,165,0.35)]"
          />
          <div>
            <h2 className="text-2xl font-bold text-text-0">Welcome to OpenPaw</h2>
            <p className="text-text-2 mt-2 max-w-md mx-auto">
              Your AI-powered internal tool factory. Build, manage, and orchestrate tools from a single operations dashboard.
            </p>
          </div>
        </div>
      ),
    },
    {
      icon: <UserPlus className="w-6 h-6" />,
      title: 'Create Admin Account',
      content: (
        <div className="space-y-4 max-w-sm mx-auto">
          <p className="text-sm text-text-2 text-center mb-2">This will be your admin account for OpenPaw.</p>
          <Input label="Username" value={username} onChange={e => setUsername(e.target.value)} placeholder="admin" error={errors.username} autoFocus />
          <Input label="Password" type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="Min 8 characters" error={errors.password} />
          <Input label="Confirm Password" type="password" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} placeholder="Re-enter password" error={errors.confirmPassword} />
        </div>
      ),
    },
    {
      icon: <Key className="w-6 h-6" />,
      title: 'API Key',
      content: (
        <div className="space-y-4 max-w-sm mx-auto">
          <div className={`flex items-center gap-3 p-3 rounded-lg border ${
            apiKeyConfigured
              ? 'bg-green-500/5 border-green-500/20'
              : 'bg-surface-2 border-border-1'
          }`}>
            {apiKeyConfigured ? (
              <CheckCircle className="w-5 h-5 text-green-400 flex-shrink-0" />
            ) : (
              <AlertTriangle className="w-5 h-5 text-amber-400 flex-shrink-0" />
            )}
            <p className={`text-sm font-medium ${apiKeyConfigured ? 'text-green-400' : 'text-amber-400'}`}>
              {apiKeyConfigured ? 'API key detected from environment' : 'No API key detected'}
            </p>
          </div>
          {!apiKeyConfigured && (
            <>
              <p className="text-sm text-text-2 text-center">
                Enter your OpenRouter API key to enable AI features. You can also set this later in Settings.
              </p>
              <Input
                label="OpenRouter API Key"
                type="password"
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                placeholder="sk-or-..."
              />
              <p className="text-xs text-text-3 text-center">
                Get a key at <span className="font-mono text-text-2">openrouter.ai/keys</span>
              </p>
            </>
          )}
          {apiKeyConfigured && (
            <p className="text-xs text-text-3 text-center">
              Your OPENROUTER_API_KEY environment variable is set. You can skip this step.
            </p>
          )}
        </div>
      ),
    },
    {
      icon: <Server className="w-6 h-6" />,
      title: 'Configure Server',
      content: (
        <div className="space-y-4 max-w-sm mx-auto">
          <Input label="Bind Address" value={bindAddress} onChange={e => setBindAddress(e.target.value)} placeholder="0.0.0.0" />
          <Input label="Port" type="number" value={port} onChange={e => setPort(e.target.value)} placeholder="8080" error={errors.port} />
        </div>
      ),
    },
    {
      icon: <PawPrint className="w-6 h-6" />,
      title: 'Meet Your Agent',
      content: (
        <div className="space-y-5 max-w-md mx-auto">
          <div className="text-center">
            <img
              src={agentAvatar}
              alt={agentName}
              className="w-20 h-20 mx-auto rounded-2xl shadow-lg shadow-black/20 mb-3"
            />
            <h2 className="text-xl font-bold text-text-0">Meet Your Agent</h2>
            <p className="text-sm text-text-2 mt-1">
              Name your AI assistant, pick an avatar, and set a goal.
            </p>
          </div>

          <Input
            label="Agent Name"
            value={agentName}
            onChange={e => setAgentName(e.target.value)}
            placeholder="Pounce"
            autoFocus
          />

          <div>
            <label className="block text-xs font-medium text-text-2 mb-1.5">Avatar</label>
            <div className="max-h-36 overflow-y-auto rounded-lg border border-border-1 bg-surface-0 p-2">
              <div className="grid grid-cols-9 gap-1.5">
                {PRESET_AVATARS.map((path, i) => (
                  <button
                    key={path}
                    onClick={() => setAgentAvatar(path)}
                    aria-label={`Select avatar ${i + 1}`}
                    aria-pressed={agentAvatar === path}
                    className={`aspect-square rounded-lg overflow-hidden border-2 transition-all cursor-pointer ${
                      agentAvatar === path
                        ? 'border-accent-primary ring-2 ring-accent-primary/20 scale-105'
                        : 'border-transparent hover:border-border-0 opacity-70 hover:opacity-100'
                    }`}
                  >
                    <img src={path} alt="" className="w-full h-full object-cover" />
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-2 mb-1.5">
              What are you building?
            </label>
            <textarea
              value={agentGoal}
              onChange={e => setAgentGoal(e.target.value)}
              placeholder="e.g. A SaaS dashboard for tracking sales metrics, A personal blog with CMS, An inventory management system..."
              rows={3}
              className="w-full rounded-lg border border-border-1 bg-surface-0 text-text-1 px-3 py-2 text-sm placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
            />
            <p className="text-[11px] text-text-3 mt-1">
              This becomes a shared goal that shapes how your agent thinks and responds. You can change it anytime.
            </p>
          </div>
        </div>
      ),
    },
  ];

  const currentStep = steps[step];

  const canGoBack = step > 0 && !setupComplete;

  return (
    <div className="min-h-screen flex items-center justify-center p-4 relative">
      <div
        className="absolute inset-0 bg-cover bg-center bg-no-repeat"
        style={{ backgroundImage: 'url(/preset-bg/bg-1.webp)' }}
      />
      <div className="absolute inset-0 bg-black/85" />

      <div className="w-full max-w-xl relative z-10">
        <div className="flex items-center justify-center gap-2 mb-8" aria-hidden="true">
          {steps.map((_, i) => (
            <div key={i} className={`h-1.5 rounded-full transition-all duration-300 ${i <= step ? 'bg-accent-primary w-10' : 'bg-surface-2 w-6'}`} />
          ))}
        </div>

        <div className="rounded-2xl border border-border-0 bg-surface-1/90 backdrop-blur-sm shadow-xl p-5 md:p-8">
          <div className="flex items-center gap-2 mb-6 justify-center">
            <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center text-accent-primary">
              {currentStep.icon}
            </div>
            <h3 className="text-sm font-medium text-text-2">
              Step {step + 1} of {steps.length}
            </h3>
          </div>

          <div className="mb-8">{currentStep.content}</div>

          <div className="flex items-center justify-between">
            {canGoBack ? (
              <Button variant="ghost" onClick={() => { setStep(step - 1); setErrors({}); }} icon={<ArrowLeft className="w-4 h-4" />}>Back</Button>
            ) : (
              <div />
            )}
            <Button onClick={handleNext} loading={loading} icon={step === steps.length - 1 ? <Check className="w-4 h-4" /> : <ArrowRight className="w-4 h-4" />}>
              {step === steps.length - 1 ? 'Complete Setup' : 'Continue'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

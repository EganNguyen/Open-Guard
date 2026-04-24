import React, { useState, useEffect } from 'react';
import { Shield } from 'lucide-react';

export function LoginPage(): JSX.Element {
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('error')) {
      setError('Authentication failed. Please try again.');
    }
  }, []);

  const handleOidcLogin = async () => {
    setLoading(true);
    setError('');
    try {
      const response = await fetch('/api/login');
      const data = await response.json();
      if (data.url) {
        window.location.href = data.url;
      } else {
        setError('OIDC login is not configured properly.');
      }
    } catch (err) {
      setError('Failed to initiate OIDC login.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-md mx-auto mt-16">
      <div className="bg-white rounded-lg shadow-lg border border-gray-200 p-8">
        <div className="flex justify-center mb-6">
          <div className="p-3 bg-blue-100 rounded-full">
            <Shield className="w-10 h-10 text-blue-600" />
          </div>
        </div>
        <h1 className="text-2xl font-bold text-gray-900 mb-2 text-center">Welcome Back</h1>
        <p className="text-gray-500 text-center mb-8">Sign in to your account using OpenGuard IAM</p>
        
        {error && (
          <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-6 text-sm">
            {error}
          </div>
        )}

        <button
          onClick={handleOidcLogin}
          disabled={loading}
          className="w-full flex items-center justify-center gap-3 py-3 px-4 bg-gray-900 text-white font-medium rounded-lg hover:bg-gray-800 transition-colors disabled:opacity-50"
        >
          {loading ? (
            <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
          ) : (
            <>
              <Shield className="w-5 h-5" />
              Sign in with OpenGuard
            </>
          )}
        </button>

        <div className="mt-8 pt-6 border-t border-gray-100">
          <p className="text-xs text-center text-gray-400">
            Secure authentication provided by OpenGuard IAM Service
          </p>
        </div>
      </div>
    </div>
  );
}
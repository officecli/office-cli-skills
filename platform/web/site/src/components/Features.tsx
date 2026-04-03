import { motion } from "motion/react";
import { Bolt, Share2, BarChart3, Activity } from "lucide-react";

export default function Features() {
  const features = [
    {
      icon: <Bolt className="text-tertiary w-10 h-10 mb-6" />,
      title: "Prompt Generation",
      description: "Describe your document in plain natural language. Our engine interprets layout, hierarchy, and data requirements automatically.",
      footer: "Available in CLI & API",
      large: true
    },
    {
      icon: <Share2 className="text-primary w-10 h-10 mb-6" />,
      title: "Workflow Integration",
      description: "Connect to GitHub Actions, CI/CD, or internal tools via our high-performance REST API.",
      large: false
    },
    {
      icon: <Activity className="text-secondary w-10 h-10 mb-6" />,
      title: "Free Starter Quota",
      description: "Get 100 generations per month free. No credit card required to start your integration.",
      large: false
    }
  ];

  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {features.map((feature, index) => (
          <motion.div 
            key={index}
            initial={{ opacity: 0, y: 20 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ delay: index * 0.1 }}
            className={`${feature.large ? 'md:col-span-2' : 'md:col-span-1'} bg-surface-low p-10 rounded-xl flex flex-col justify-between group hover:bg-surface-high transition-all border border-white/5`}
          >
            <div>
              {feature.icon}
              <h3 className="font-headline text-3xl font-bold text-white mb-4">{feature.title}</h3>
              <p className="text-outline-variant leading-relaxed">{feature.description}</p>
            </div>
            {feature.footer && (
              <div className="mt-8 border-t border-outline-variant/10 pt-6">
                <span className="text-xs font-headline uppercase text-tertiary tracking-widest">{feature.footer}</span>
              </div>
            )}
          </motion.div>
        ))}
        
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.3 }}
          className="md:col-span-4 bg-surface-low p-10 rounded-xl flex flex-col md:flex-row items-center gap-12 group hover:bg-surface-high transition-all border border-white/5"
        >
          <div className="flex-1">
            <h3 className="font-headline text-3xl font-bold text-white mb-4">Unified Infrastructure Management</h3>
            <p className="text-outline-variant max-w-2xl">Monitor usage, manage API keys, and track generation success rates through a centralized dashboard or direct terminal readouts.</p>
          </div>
          <div className="hidden lg:flex w-48 h-24 bg-surface-high rounded border border-outline-variant/10 items-center justify-center">
            <div className="flex gap-2 items-end">
              <motion.div animate={{ height: [20, 40, 20] }} transition={{ repeat: Infinity, duration: 2 }} className="w-2 bg-tertiary h-8"></motion.div>
              <motion.div animate={{ height: [30, 60, 30] }} transition={{ repeat: Infinity, duration: 2.5 }} className="w-2 bg-tertiary h-12"></motion.div>
              <motion.div animate={{ height: [15, 30, 15] }} transition={{ repeat: Infinity, duration: 1.8 }} className="w-2 bg-tertiary h-6"></motion.div>
              <motion.div animate={{ height: [25, 50, 25] }} transition={{ repeat: Infinity, duration: 2.2 }} className="w-2 bg-tertiary h-10"></motion.div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  );
}
